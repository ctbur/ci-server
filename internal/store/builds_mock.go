package store

import (
	"context"
	"sort"
	"time"
)

type MockBuildStore struct {
	repos  map[uint64]Repo
	builds map[uint64]Build
}

func NewMockBuildStore() *MockBuildStore {
	return &MockBuildStore{
		repos:  make(map[uint64]Repo),
		builds: make(map[uint64]Build),
	}
}

func (s MockBuildStore) CreateRepoIfNotExists(ctx context.Context, repo RepoMeta) error {
	for _, r := range s.repos {
		if r.Owner == repo.Owner && r.Name == repo.Name {
			return nil
		}
	}

	var newID uint64 = uint64(len(s.repos) + 1)

	s.repos[newID] = Repo{
		ID: newID,
		RepoMeta: RepoMeta{
			Owner: repo.Owner,
			Name:  repo.Name,
		},
		RepoState: RepoState{
			BuildCounter: 0,
		},
	}

	return nil
}

func (s MockBuildStore) CountRepos(ctx context.Context) (uint64, error) {
	return uint64(len(s.repos)), nil
}

func (s MockBuildStore) CreateBuild(
	ctx context.Context, repoOwner, repoName string, build BuildMeta, ts time.Time,
) (uint64, error) {
	var repoID uint64
	for id, r := range s.repos {
		if r.Owner == repoOwner && r.Name == repoName {
			repoID = id
			break
		}
	}
	if repoID == 0 {
		return 0, ErrNoBuild
	}

	repo := s.repos[repoID]
	repo.BuildCounter++
	s.repos[repoID] = repo

	newBuildID := uint64(len(s.builds) + 1)
	s.builds[newBuildID] = Build{
		ID:        newBuildID,
		RepoID:    repoID,
		Number:    repo.BuildCounter,
		BuildMeta: build,
		BuildState: BuildState{
			Created:  ts,
			Started:  nil,
			Finished: nil,
			Result:   nil,
		},
	}

	return newBuildID, nil
}

func (s MockBuildStore) UpdateBuildState(ctx context.Context, buildID uint64, state BuildState) error {
	build, exists := s.builds[buildID]
	if !exists {
		// TODO: return error in real implementation
		return nil
	}

	build.BuildState = state
	s.builds[buildID] = build
	return nil
}

func (s MockBuildStore) GetBuild(ctx context.Context, buildID uint64) (*BuildWithRepoMeta, error) {
	build, exists := s.builds[buildID]
	if !exists {
		return nil, ErrNoBuild
	}

	repo, repoExists := s.repos[build.RepoID]
	if !repoExists {
		return nil, ErrNoBuild
	}

	return &BuildWithRepoMeta{
		Build:    build,
		RepoMeta: repo.RepoMeta,
	}, nil
}

func (s MockBuildStore) ListBuilds(ctx context.Context) ([]BuildWithRepoMeta, error) {
	var builds []BuildWithRepoMeta
	for _, build := range s.builds {
		repo, repoExists := s.repos[build.RepoID]
		if !repoExists {
			continue
		}
		builds = append(builds, BuildWithRepoMeta{
			Build:    build,
			RepoMeta: repo.RepoMeta,
		})
	}

	sort.Slice(builds, func(i, j int) bool {
		return builds[i].Created.After(builds[j].Created)
	})

	return builds, nil
}

func (s MockBuildStore) GetPendingBuilds(ctx context.Context) ([]BuildWithRepoMeta, error) {
	var pendingBuilds []BuildWithRepoMeta
	for _, build := range s.builds {
		if build.Started == nil && build.Finished == nil && build.Result == nil {
			repo, repoExists := s.repos[build.RepoID]
			if !repoExists {
				continue
			}
			pendingBuilds = append(pendingBuilds, BuildWithRepoMeta{
				Build:    build,
				RepoMeta: repo.RepoMeta,
			})
		}
	}

	sort.Slice(pendingBuilds, func(i, j int) bool {
		return pendingBuilds[i].Created.Before(pendingBuilds[j].Created)
	})

	return pendingBuilds, nil
}
