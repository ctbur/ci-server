package store

import (
	"context"
	"testing"
	"time"

	"github.com/ctbur/ci-server/v2/internal/assert"
)

func TestBuildStore(t *testing.T) {
	ctx := context.Background()

	err, pool, cleanup := StartTestDatabase(t, ctx, "../../")
	if err != nil {
		t.Fatalf("Failed to set up database: %v", err)
	}
	defer cleanup()

	s := NewPGStore(pool)

	t.Run("Create new repositories", func(t *testing.T) {
		err := s.CreateRepoIfNotExists(ctx, Repo{
			Owner: "owner",
			Name:  "repo1",
		})
		assert.NoError(t, err, "Failed to create owner/repo1").Fatal()

		err = s.CreateRepoIfNotExists(ctx, Repo{
			Owner: "owner",
			Name:  "repo2",
		})
		assert.NoError(t, err, "Failed to create owner/repo2").Fatal()

		err = s.CreateRepoIfNotExists(ctx, Repo{
			Owner: "owner",
			Name:  "repo1",
		})
		assert.NoError(t, err, "Failed to create owner/repo1 the second time (should be no-op)")

		// There should be exactly 2 repositories now
		numRepos, err := s.CountRepos(ctx)
		assert.NoError(t, err, "Failed to count repositories").Fatal()
		assert.Equal(t, numRepos, 2, "There should be exactly 2 repositories").Fatal()
	})

	t.Run("Create and retrieve builds", func(t *testing.T) {
		// Create builds
		r1b1 := BuildMeta{
			Link:      "https://github.com/owner/repos1/b1",
			Ref:       "ref_r1b1",
			CommitSHA: "000011",
			Message:   "message_r1b1",
		}
		r1b1ID, err := s.CreateBuild(ctx, "owner", "repo1", r1b1, time.UnixMilli(11))
		assert.NoError(t, err, "Failed to create build").Fatal()
		assert.Equal(t, r1b1ID, 1, "Incorrect ID for build").Fatal()

		r1b2 := BuildMeta{
			Link:      "https://github.com/owner/repos1/b2",
			Ref:       "ref_r1b2",
			CommitSHA: "000012",
			Message:   "message_r1b2",
		}
		r1b2ID, err := s.CreateBuild(ctx, "owner", "repo1", r1b2, time.UnixMilli(12))
		assert.NoError(t, err, "Failed to create build").Fatal()
		assert.Equal(t, r1b2ID, 2, "Incorrect ID for build").Fatal()

		r2b1 := BuildMeta{
			Link:      "https://github.com/owner/repos2/b1",
			Ref:       "ref_r2b1",
			CommitSHA: "000021",
			Message:   "message_r2b1",
		}
		r2b1ID, err := s.CreateBuild(ctx, "owner", "repo2", r2b1, time.UnixMilli(21))
		assert.NoError(t, err, "Failed to create build").Fatal()
		assert.Equal(t, r2b1ID, 3, "Incorrect ID for build").Fatal()

		r2b2 := BuildMeta{
			Link:      "https://github.com/owner/repos2/b2",
			Ref:       "ref_r2b2",
			CommitSHA: "000022",
			Message:   "message_r2b2",
		}
		r2b2ID, err := s.CreateBuild(ctx, "owner", "repo2", r2b2, time.UnixMilli(22))
		assert.NoError(t, err, "Failed to create build").Fatal()
		assert.Equal(t, r2b2ID, 4, "Incorrect ID for build").Fatal()

		// Test getting single build
		r2b2got, err := s.GetBuild(ctx, r2b2ID)
		assert.NoError(t, err, "Failed to get build").Fatal()

		r2b2want := Build{
			ID:     4,
			RepoID: 2,
			Number: 2,
			BuildMeta: BuildMeta{
				Link:      "https://github.com/owner/repos2/b2",
				Ref:       "ref_r2b2",
				CommitSHA: "000022",
				Message:   "message_r2b2",
			},
			Created:  time.UnixMilli(22),
			Started:  nil,
			Finished: nil,
			Result:   nil,
			Repo: Repo{
				Owner: "owner",
				Name:  "repo2",
			},
		}
		assert.Equal(t, *r2b2got, r2b2want, "Unexpected build retrieved").Fatal()

		_, err = s.GetBuild(ctx, 100)
		assert.ErrorIs(t, err, ErrNoBuild, "Incorrect error for non-existent build").Fatal()

		// Test listing latest builds
		builds, err := s.ListBuilds(ctx, nil, 4)
		assert.NoError(t, err, "Failed to list builds").Fatal()
		assert.DeepEqual(t,
			[]uint64{builds[0].ID, builds[1].ID, builds[2].ID, builds[3].ID},
			// Created desc
			[]uint64{4, 3, 2, 1},
			"Incorrect build IDs",
		).Fatal()
		assert.Equal(t, builds[0], r2b2want, "Unexpected build retrieved")

		// Test listing builds with beforeID and limit
		beforeID := uint64(3)
		builds, err = s.ListBuilds(ctx, &beforeID, 3)
		assert.NoError(t, err, "Failed to list builds").Fatal()
		assert.DeepEqual(t,
			[]uint64{builds[0].ID, builds[1].ID},
			// Created desc
			[]uint64{3, 2},
			"Incorrect build IDs",
		).Fatal()
	})

	t.Run("Get pending and running builds", func(t *testing.T) {
		pendingBuilds, err := s.GetPendingBuilds(ctx)
		assert.NoError(t, err, "Failed to get pending builds")
		assert.Equal(t, len(pendingBuilds), 4, "Incorrect number of pending builds")
		// Should be in order of creation
		assert.Equal(t, pendingBuilds[0].ID, 1, "Incorrect order of pending builds")
		assert.Equal(t, pendingBuilds[1].ID, 2, "Incorrect order of pending builds")
		assert.Equal(t, pendingBuilds[2].ID, 3, "Incorrect order of pending builds")
		assert.Equal(t, pendingBuilds[3].ID, 4, "Incorrect order of pending builds")

		runningBuilders, err := s.ListBuilders(ctx)
		assert.NoError(t, err, "Failed to get running builds")
		assert.Equal(t, len(runningBuilders), 0, "Got running builders where there shold be none")
	})

	t.Run("Start and finish first build of each repo", func(t *testing.T) {
		// Finish r1b1 and cache results
		s.StartBuild(ctx, 1, time.UnixMilli(1011), 10011, nil)
		s.FinishBuild(ctx, 1, time.UnixMilli(2011), BuildResultSuccess, true)
		// Finish r2b1 without caching results
		s.StartBuild(ctx, 3, time.UnixMilli(1021), 10021, nil)
		s.FinishBuild(ctx, 3, time.UnixMilli(2021), BuildResultSuccess, false)

		pendingBuilds, err := s.GetPendingBuilds(ctx)
		assert.NoError(t, err, "Failed to get pending builds")
		assert.Equal(t, len(pendingBuilds), 2, "Incorrect number of pending builds")
		// Should be in order of creation
		assert.Equal(t, pendingBuilds[0].ID, 2, "Incorrect order of pending builds")
		assert.Equal(t, pendingBuilds[1].ID, 4, "Incorrect order of pending builds")
		assert.Equal(t, *pendingBuilds[0].CacheID, 1, "Incorrect cache ID")
		assert.Equal(t, pendingBuilds[1].CacheID, nil, "Incorrect cache ID")

		runningBuilders, err := s.ListBuilders(ctx)
		assert.NoError(t, err, "Failed to get running builds")
		assert.Equal(t, len(runningBuilders), 0, "Got running builders where there shold be none")

		// Check start and finish times
		r1r1got, err := s.GetBuild(ctx, 1)
		assert.NoError(t, err, "Failed to get build")
		assert.Equal(t, *r1r1got.Started, time.UnixMilli(1011), "Incorrect start time")
		assert.Equal(t, *r1r1got.Finished, time.UnixMilli(2011), "Incorrect start time")

		r2r1got, err := s.GetBuild(ctx, 3)
		assert.NoError(t, err, "Failed to get build")
		assert.Equal(t, *r2r1got.Started, time.UnixMilli(1021), "Incorrect start time")
		assert.Equal(t, *r2r1got.Finished, time.UnixMilli(2021), "Incorrect start time")
	})

	t.Run("Start second build of each repo", func(t *testing.T) {
		// Start r1b2 with cache
		cacheID := uint64(1)
		s.StartBuild(ctx, 2, time.UnixMilli(1012), 10012, &cacheID)
		// Start r1b2 without cache
		s.StartBuild(ctx, 4, time.UnixMilli(1022), 10022, nil)

		pendingBuilds, err := s.GetPendingBuilds(ctx)
		assert.NoError(t, err, "Failed to get pending builds")
		assert.Equal(t, len(pendingBuilds), 0, "Got pending builds when there should be none")

		builders, err := s.ListBuilders(ctx)
		assert.NoError(t, err, "Failed to list builders")
		assert.Equal(t, len(builders), 2, "Incorrect number of builders")
		// Check cache ID in order of build ID
		assert.Equal(t, *builders[0].CacheID, 1, "Incorrect cache ID")
		assert.Equal(t, builders[1].CacheID, nil, "Incorrect cache ID")

		// Get list of build dirs to retain
		buildIDs, err := s.ListBuildDirsInUse(ctx)
		assert.NoError(t, err, "Failed to list build dirs in use")
		assert.DeepEqual(t, buildIDs, []uint64{1, 2, 4}, "Incorrect build dirs in use")
	})
}
