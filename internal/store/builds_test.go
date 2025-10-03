package store

import (
	"context"
	"testing"
	"time"

	"github.com/ctbur/ci-server/v2/internal/assert"
)

type BuildStoreImpl interface {
	CreateRepoIfNotExists(ctx context.Context, repo RepoMeta) error
	CountRepos(ctx context.Context) (uint64, error)
	CreateBuild(ctx context.Context, repoOwner, repoName string, build BuildMeta, ts time.Time) (uint64, error)
	MarkBuildStarted(ctx context.Context, buildID uint64, started time.Time) error
	MarkBuildFinished(ctx context.Context, buildID uint64, finished time.Time, result BuildResult) error
	GetBuild(ctx context.Context, buildID uint64) (*BuildWithRepoMeta, error)
	ListBuilds(ctx context.Context) ([]BuildWithRepoMeta, error)
	GetPendingBuilds(ctx context.Context) ([]BuildWithRepoMeta, error)
}

func TestBuildStore(t *testing.T) {
	ctx := context.Background()

	err, pool, cleanup := StartTestDatabase(t, ctx, "../../")
	if err != nil {
		t.Fatalf("Failed to set up database: %v", err)
	}
	defer cleanup()

	s := NewPGStore(pool)
	BuildStoreTest(t, ctx, s)
}

func TestBuildStoreMock(t *testing.T) {
	ctx := context.Background()
	BuildStoreTest(t, ctx, NewMockBuildStore())
}

func BuildStoreTest(t *testing.T, ctx context.Context, s BuildStoreImpl) {
	t.Run("Create new repositories", func(t *testing.T) {
		err := s.CreateRepoIfNotExists(ctx, RepoMeta{
			Owner: "owner",
			Name:  "repo1",
		})
		assert.NoError(t, err, "Failed to create owner/repo1")

		err = s.CreateRepoIfNotExists(ctx, RepoMeta{
			Owner: "owner",
			Name:  "repo2",
		})
		assert.NoError(t, err, "Failed to create owner/repo2")

		err = s.CreateRepoIfNotExists(ctx, RepoMeta{
			Owner: "owner",
			Name:  "repo1",
		})
		assert.NoError(t, err, "Failed to create owner/repo1 the second time (should be no-op)")

		// There should be exactly 2 repositories now
		numRepos, err := s.CountRepos(ctx)
		assert.NoError(t, err, "Failed to count repositories")
		assert.Equal(t, numRepos, 2, "There should be exactly 2 repositories")
	})

	// Defined here because it's used in two tests
	r2b2want := BuildWithRepoMeta{
		Build: Build{
			ID:     4,
			RepoID: 2,
			Number: 2,
			BuildMeta: BuildMeta{
				Link:      "https://github.com/owner/repos2/b2",
				Ref:       "ref_r2b2",
				CommitSHA: "000022",
				Message:   "message_r2b2",
			},
			BuildState: BuildState{
				Created:  time.UnixMilli(1758910000022),
				Started:  nil,
				Finished: nil,
				Result:   nil,
			},
		},
		RepoMeta: RepoMeta{
			Owner: "owner",
			Name:  "repo2",
		},
	}

	t.Run("Create and retrieve builds", func(t *testing.T) {
		// Create builds
		r1b1 := BuildMeta{
			Link:      "https://github.com/owner/repos1/b1",
			Ref:       "ref_r1b1",
			CommitSHA: "000011",
			Message:   "message_r1b1",
		}
		r1b1ID, err := s.CreateBuild(ctx, "owner", "repo1", r1b1, time.UnixMilli(1758910000011))
		assert.NoError(t, err, "Failed to create build")
		assert.Equal(t, r1b1ID, 1, "Incorrect ID for build")

		r1b2 := BuildMeta{
			Link:      "https://github.com/owner/repos1/b2",
			Ref:       "ref_r1b2",
			CommitSHA: "000012",
			Message:   "message_r1b2",
		}
		r1b2ID, err := s.CreateBuild(ctx, "owner", "repo1", r1b2, time.UnixMilli(1758910000012))
		assert.NoError(t, err, "Failed to create build")
		assert.Equal(t, r1b2ID, 2, "Incorrect ID for build")

		r2b1 := BuildMeta{
			Link:      "https://github.com/owner/repos2/b1",
			Ref:       "ref_r2b1",
			CommitSHA: "000021",
			Message:   "message_r2b1",
		}
		r2b1ID, err := s.CreateBuild(ctx, "owner", "repo2", r2b1, time.UnixMilli(1758910000021))
		assert.NoError(t, err, "Failed to create build")
		assert.Equal(t, r2b1ID, 3, "Incorrect ID for build")

		r2b2 := BuildMeta{
			Link:      "https://github.com/owner/repos2/b2",
			Ref:       "ref_r2b2",
			CommitSHA: "000022",
			Message:   "message_r2b2",
		}
		r2b2ID, err := s.CreateBuild(ctx, "owner", "repo2", r2b2, time.UnixMilli(1758910000022))
		assert.NoError(t, err, "Failed to create build")
		assert.Equal(t, r2b2ID, 4, "Incorrect ID for build")

		// Test getting single build
		r2b2got, err := s.GetBuild(ctx, r2b2ID)
		assert.NoError(t, err, "Failed to get build")
		assert.Equal(t, *r2b2got, r2b2want, "Unexpected build retrieved")

		_, err = s.GetBuild(ctx, 100)
		assert.ErrorIs(t, err, ErrNoBuild, "Incorrect error for non-existent build")

		// Test listing builds
		builds, err := s.ListBuilds(ctx)
		assert.NoError(t, err, "Failed to list builds")
		assert.DeepEqual(t,
			[]uint64{builds[0].ID, builds[1].ID, builds[2].ID, builds[3].ID},
			// Created desc
			[]uint64{4, 3, 2, 1},
			"Incorrect build IDs",
		)
		assert.Equal(t, builds[0], r2b2want, "Unexpected build retrieved")
	})

	t.Run("Update build states and get pending builds", func(t *testing.T) {
		// Update specific build state
		started := time.UnixMilli(1758910001021)
		finished := time.UnixMilli(1758910002021)
		result := BuildResultSuccess
		r2b1want := BuildState{
			Created:  time.UnixMilli(1758910000021),
			Started:  &started,
			Finished: &finished,
			Result:   &result,
		}

		s.MarkBuildStarted(ctx, 3, *r2b1want.Started)
		s.MarkBuildFinished(ctx, 3, *r2b1want.Finished, *r2b1want.Result)

		r2b1got, err := s.GetBuild(ctx, 3)
		assert.NoError(t, err, "Failed to get build")
		assert.DeepEqual(t, r2b1got.BuildState, r2b1want, "Unexpected build state retrieved")

		// List pending builds
		builds, err := s.GetPendingBuilds(ctx)
		assert.NoError(t, err, "Failed to list pending builds")
		assert.DeepEqual(t,
			[]uint64{builds[0].ID, builds[1].ID, builds[2].ID},
			// Created asc
			[]uint64{1, 2, 4},
			"Incorrect build IDs",
		)
		assert.Equal(t, builds[2], r2b2want, "Unexpected pending build retrieved")
	})
}
