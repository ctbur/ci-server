package webhook

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ctbur/ci-server/v2/internal/assert"
	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/github"
	"github.com/ctbur/ci-server/v2/internal/store"
)

var testWebhookSecret = "sLBCQgxE29C1mgA0EVt4n2RMBPdH6iq1" // Not a valid secret, just for testing

var baseHeader http.Header = map[string][]string{
	"Accept":                                 {"*/*"},
	"Content-Type":                           {"application/json"},
	"User-Agent":                             {"GitHub-Hookshot/1fd3ecb"},
	"X-Github-Delivery":                      {"05974692-c799-11f0-8999-843b48c89eba"},
	"X-Github-Event":                         {"push"},
	"X-Github-Hook-ID":                       {"574845133"},
	"X-Github-Hook-Installation-Target-ID":   {"2105522"},
	"X-Github-Hook-Installation-Target-Type": {"integration"},
	"X-Hub-Signature-256":                    {"sha256=0387235249a69255fdc2eb64205892f619ecf4cffdf87895366f808ab7516a41"},
}

const pushPayload = `{"ref":"refs/heads/main","before":"26a2bf7cb1bf84c5d18a23ec132eab935386d6ab","after":"8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4","repository":{"id":926103085,"node_id":"R_kgDONzM2LQ","name":"ci-server","full_name":"ctbur/ci-server","private":false,"owner":{"name":"ctbur","email":"41328971+ctbur@users.noreply.github.com","login":"ctbur","id":41328971,"node_id":"MDQ6VXNlcjQxMzI4OTcx","avatar_url":"https://avatars.githubusercontent.com/u/41328971?v=4","gravatar_id":"","url":"https://api.github.com/users/ctbur","html_url":"https://github.com/ctbur","followers_url":"https://api.github.com/users/ctbur/followers","following_url":"https://api.github.com/users/ctbur/following{/other_user}","gists_url":"https://api.github.com/users/ctbur/gists{/gist_id}","starred_url":"https://api.github.com/users/ctbur/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/ctbur/subscriptions","organizations_url":"https://api.github.com/users/ctbur/orgs","repos_url":"https://api.github.com/users/ctbur/repos","events_url":"https://api.github.com/users/ctbur/events{/privacy}","received_events_url":"https://api.github.com/users/ctbur/received_events","type":"User","user_view_type":"public","site_admin":false},"html_url":"https://github.com/ctbur/ci-server","description":null,"fork":false,"url":"https://api.github.com/repos/ctbur/ci-server","forks_url":"https://api.github.com/repos/ctbur/ci-server/forks","keys_url":"https://api.github.com/repos/ctbur/ci-server/keys{/key_id}","collaborators_url":"https://api.github.com/repos/ctbur/ci-server/collaborators{/collaborator}","teams_url":"https://api.github.com/repos/ctbur/ci-server/teams","hooks_url":"https://api.github.com/repos/ctbur/ci-server/hooks","issue_events_url":"https://api.github.com/repos/ctbur/ci-server/issues/events{/number}","events_url":"https://api.github.com/repos/ctbur/ci-server/events","assignees_url":"https://api.github.com/repos/ctbur/ci-server/assignees{/user}","branches_url":"https://api.github.com/repos/ctbur/ci-server/branches{/branch}","tags_url":"https://api.github.com/repos/ctbur/ci-server/tags","blobs_url":"https://api.github.com/repos/ctbur/ci-server/git/blobs{/sha}","git_tags_url":"https://api.github.com/repos/ctbur/ci-server/git/tags{/sha}","git_refs_url":"https://api.github.com/repos/ctbur/ci-server/git/refs{/sha}","trees_url":"https://api.github.com/repos/ctbur/ci-server/git/trees{/sha}","statuses_url":"https://api.github.com/repos/ctbur/ci-server/statuses/{sha}","languages_url":"https://api.github.com/repos/ctbur/ci-server/languages","stargazers_url":"https://api.github.com/repos/ctbur/ci-server/stargazers","contributors_url":"https://api.github.com/repos/ctbur/ci-server/contributors","subscribers_url":"https://api.github.com/repos/ctbur/ci-server/subscribers","subscription_url":"https://api.github.com/repos/ctbur/ci-server/subscription","commits_url":"https://api.github.com/repos/ctbur/ci-server/commits{/sha}","git_commits_url":"https://api.github.com/repos/ctbur/ci-server/git/commits{/sha}","comments_url":"https://api.github.com/repos/ctbur/ci-server/comments{/number}","issue_comment_url":"https://api.github.com/repos/ctbur/ci-server/issues/comments{/number}","contents_url":"https://api.github.com/repos/ctbur/ci-server/contents/{+path}","compare_url":"https://api.github.com/repos/ctbur/ci-server/compare/{base}...{head}","merges_url":"https://api.github.com/repos/ctbur/ci-server/merges","archive_url":"https://api.github.com/repos/ctbur/ci-server/{archive_format}{/ref}","downloads_url":"https://api.github.com/repos/ctbur/ci-server/downloads","issues_url":"https://api.github.com/repos/ctbur/ci-server/issues{/number}","pulls_url":"https://api.github.com/repos/ctbur/ci-server/pulls{/number}","milestones_url":"https://api.github.com/repos/ctbur/ci-server/milestones{/number}","notifications_url":"https://api.github.com/repos/ctbur/ci-server/notifications{?since,all,participating}","labels_url":"https://api.github.com/repos/ctbur/ci-server/labels{/name}","releases_url":"https://api.github.com/repos/ctbur/ci-server/releases{/id}","deployments_url":"https://api.github.com/repos/ctbur/ci-server/deployments","created_at":1738508779,"updated_at":"2025-11-22T09:27:24Z","pushed_at":1763812047,"git_url":"git://github.com/ctbur/ci-server.git","ssh_url":"git@github.com:ctbur/ci-server.git","clone_url":"https://github.com/ctbur/ci-server.git","svn_url":"https://github.com/ctbur/ci-server","homepage":null,"size":209,"stargazers_count":0,"watchers_count":0,"language":"Go","has_issues":true,"has_projects":true,"has_downloads":true,"has_wiki":false,"has_pages":false,"has_discussions":false,"forks_count":0,"mirror_url":null,"archived":false,"disabled":false,"open_issues_count":0,"license":null,"allow_forking":true,"is_template":false,"web_commit_signoff_required":false,"topics":[],"visibility":"public","forks":0,"open_issues":0,"watchers":0,"default_branch":"main","stargazers":0,"master_branch":"main"},"pusher":{"name":"ctbur","email":"41328971+ctbur@users.noreply.github.com"},"sender":{"login":"ctbur","id":41328971,"node_id":"MDQ6VXNlcjQxMzI4OTcx","avatar_url":"https://avatars.githubusercontent.com/u/41328971?v=4","gravatar_id":"","url":"https://api.github.com/users/ctbur","html_url":"https://github.com/ctbur","followers_url":"https://api.github.com/users/ctbur/followers","following_url":"https://api.github.com/users/ctbur/following{/other_user}","gists_url":"https://api.github.com/users/ctbur/gists{/gist_id}","starred_url":"https://api.github.com/users/ctbur/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/ctbur/subscriptions","organizations_url":"https://api.github.com/users/ctbur/orgs","repos_url":"https://api.github.com/users/ctbur/repos","events_url":"https://api.github.com/users/ctbur/events{/privacy}","received_events_url":"https://api.github.com/users/ctbur/received_events","type":"User","user_view_type":"public","site_admin":false},"installation":{"id":92907551,"node_id":"MDIzOkludGVncmF0aW9uSW5zdGFsbGF0aW9uOTI5MDc1NTE="},"created":false,"deleted":false,"forced":false,"base_ref":null,"compare":"https://github.com/ctbur/ci-server/compare/26a2bf7cb1bf...8caeb971b6ff","commits":[{"id":"8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4","tree_id":"1b0be37edecd04276ab165d7920c27a404643082","distinct":true,"message":"Fix app token and improve dev setup (#21)","timestamp":"2025-11-22T12:47:27+01:00","url":"https://github.com/ctbur/ci-server/commit/8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4","author":{"name":"Cyrill Burgener","email":"41328971+ctbur@users.noreply.github.com","date":"2025-11-22T12:47:27+01:00","username":"ctbur"},"committer":{"name":"GitHub","email":"noreply@github.com","date":"2025-11-22T12:47:27+01:00","username":"web-flow"},"added":[".zed/debug.json"],"removed":["ci-config.toml","users.htpasswd"],"modified":[".gitignore","Makefile","cmd/server/main.go","internal/github/github.go","scripts/manual-webhook.sh"]}],"head_commit":{"id":"8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4","tree_id":"1b0be37edecd04276ab165d7920c27a404643082","distinct":true,"message":"Fix app token and improve dev setup (#21)","timestamp":"2025-11-22T12:47:27+01:00","url":"https://github.com/ctbur/ci-server/commit/8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4","author":{"name":"Cyrill Burgener","email":"41328971+ctbur@users.noreply.github.com","date":"2025-11-22T12:47:27+01:00","username":"ctbur"},"committer":{"name":"GitHub","email":"noreply@github.com","date":"2025-11-22T12:47:27+01:00","username":"web-flow"},"added":[".zed/debug.json"],"removed":["ci-config.toml","users.htpasswd"],"modified":[".gitignore","Makefile","cmd/server/main.go","internal/github/github.go","scripts/manual-webhook.sh"]}}`

const branchDeletePayload = `{"ref":"refs/heads/fix-installation-token","before":"5209412abee89ed9774d7e3779a91964801d6790","after":"0000000000000000000000000000000000000000","repository":{"id":926103085,"node_id":"R_kgDONzM2LQ","name":"ci-server","full_name":"ctbur/ci-server","private":false,"owner":{"name":"ctbur","email":"41328971+ctbur@users.noreply.github.com","login":"ctbur","id":41328971,"node_id":"MDQ6VXNlcjQxMzI4OTcx","avatar_url":"https://avatars.githubusercontent.com/u/41328971?v=4","gravatar_id":"","url":"https://api.github.com/users/ctbur","html_url":"https://github.com/ctbur","followers_url":"https://api.github.com/users/ctbur/followers","following_url":"https://api.github.com/users/ctbur/following{/other_user}","gists_url":"https://api.github.com/users/ctbur/gists{/gist_id}","starred_url":"https://api.github.com/users/ctbur/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/ctbur/subscriptions","organizations_url":"https://api.github.com/users/ctbur/orgs","repos_url":"https://api.github.com/users/ctbur/repos","events_url":"https://api.github.com/users/ctbur/events{/privacy}","received_events_url":"https://api.github.com/users/ctbur/received_events","type":"User","user_view_type":"public","site_admin":false},"html_url":"https://github.com/ctbur/ci-server","description":null,"fork":false,"url":"https://api.github.com/repos/ctbur/ci-server","forks_url":"https://api.github.com/repos/ctbur/ci-server/forks","keys_url":"https://api.github.com/repos/ctbur/ci-server/keys{/key_id}","collaborators_url":"https://api.github.com/repos/ctbur/ci-server/collaborators{/collaborator}","teams_url":"https://api.github.com/repos/ctbur/ci-server/teams","hooks_url":"https://api.github.com/repos/ctbur/ci-server/hooks","issue_events_url":"https://api.github.com/repos/ctbur/ci-server/issues/events{/number}","events_url":"https://api.github.com/repos/ctbur/ci-server/events","assignees_url":"https://api.github.com/repos/ctbur/ci-server/assignees{/user}","branches_url":"https://api.github.com/repos/ctbur/ci-server/branches{/branch}","tags_url":"https://api.github.com/repos/ctbur/ci-server/tags","blobs_url":"https://api.github.com/repos/ctbur/ci-server/git/blobs{/sha}","git_tags_url":"https://api.github.com/repos/ctbur/ci-server/git/tags{/sha}","git_refs_url":"https://api.github.com/repos/ctbur/ci-server/git/refs{/sha}","trees_url":"https://api.github.com/repos/ctbur/ci-server/git/trees{/sha}","statuses_url":"https://api.github.com/repos/ctbur/ci-server/statuses/{sha}","languages_url":"https://api.github.com/repos/ctbur/ci-server/languages","stargazers_url":"https://api.github.com/repos/ctbur/ci-server/stargazers","contributors_url":"https://api.github.com/repos/ctbur/ci-server/contributors","subscribers_url":"https://api.github.com/repos/ctbur/ci-server/subscribers","subscription_url":"https://api.github.com/repos/ctbur/ci-server/subscription","commits_url":"https://api.github.com/repos/ctbur/ci-server/commits{/sha}","git_commits_url":"https://api.github.com/repos/ctbur/ci-server/git/commits{/sha}","comments_url":"https://api.github.com/repos/ctbur/ci-server/comments{/number}","issue_comment_url":"https://api.github.com/repos/ctbur/ci-server/issues/comments{/number}","contents_url":"https://api.github.com/repos/ctbur/ci-server/contents/{+path}","compare_url":"https://api.github.com/repos/ctbur/ci-server/compare/{base}...{head}","merges_url":"https://api.github.com/repos/ctbur/ci-server/merges","archive_url":"https://api.github.com/repos/ctbur/ci-server/{archive_format}{/ref}","downloads_url":"https://api.github.com/repos/ctbur/ci-server/downloads","issues_url":"https://api.github.com/repos/ctbur/ci-server/issues{/number}","pulls_url":"https://api.github.com/repos/ctbur/ci-server/pulls{/number}","milestones_url":"https://api.github.com/repos/ctbur/ci-server/milestones{/number}","notifications_url":"https://api.github.com/repos/ctbur/ci-server/notifications{?since,all,participating}","labels_url":"https://api.github.com/repos/ctbur/ci-server/labels{/name}","releases_url":"https://api.github.com/repos/ctbur/ci-server/releases{/id}","deployments_url":"https://api.github.com/repos/ctbur/ci-server/deployments","created_at":1738508779,"updated_at":"2025-11-22T09:27:24Z","pushed_at":1763812048,"git_url":"git://github.com/ctbur/ci-server.git","ssh_url":"git@github.com:ctbur/ci-server.git","clone_url":"https://github.com/ctbur/ci-server.git","svn_url":"https://github.com/ctbur/ci-server","homepage":null,"size":209,"stargazers_count":0,"watchers_count":0,"language":"Go","has_issues":true,"has_projects":true,"has_downloads":true,"has_wiki":false,"has_pages":false,"has_discussions":false,"forks_count":0,"mirror_url":null,"archived":false,"disabled":false,"open_issues_count":0,"license":null,"allow_forking":true,"is_template":false,"web_commit_signoff_required":false,"topics":[],"visibility":"public","forks":0,"open_issues":0,"watchers":0,"default_branch":"main","stargazers":0,"master_branch":"main"},"pusher":{"name":"ctbur","email":"41328971+ctbur@users.noreply.github.com"},"sender":{"login":"ctbur","id":41328971,"node_id":"MDQ6VXNlcjQxMzI4OTcx","avatar_url":"https://avatars.githubusercontent.com/u/41328971?v=4","gravatar_id":"","url":"https://api.github.com/users/ctbur","html_url":"https://github.com/ctbur","followers_url":"https://api.github.com/users/ctbur/followers","following_url":"https://api.github.com/users/ctbur/following{/other_user}","gists_url":"https://api.github.com/users/ctbur/gists{/gist_id}","starred_url":"https://api.github.com/users/ctbur/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/ctbur/subscriptions","organizations_url":"https://api.github.com/users/ctbur/orgs","repos_url":"https://api.github.com/users/ctbur/repos","events_url":"https://api.github.com/users/ctbur/events{/privacy}","received_events_url":"https://api.github.com/users/ctbur/received_events","type":"User","user_view_type":"public","site_admin":false},"installation":{"id":92907551,"node_id":"MDIzOkludGVncmF0aW9uSW5zdGFsbGF0aW9uOTI5MDc1NTE="},"created":false,"deleted":true,"forced":false,"base_ref":null,"compare":"https://github.com/ctbur/ci-server/compare/5209412abee8...000000000000","commits":[],"head_commit":null}`

func withHeader(header http.Header, key, value string) http.Header {
	h := header.Clone()
	h.Set(key, value)
	return h
}

func withoutHeader(h http.Header, key string) http.Header {
	clone := h.Clone()
	clone.Del(key)
	return clone
}

func headerDel(header http.Header, key string) http.Header {
	h := header.Clone()
	h.Del(key)
	return h
}

type MockBuildCreator struct {
	build *MockBuild
}

type MockBuild struct {
	InstallationID      github.InstallationID
	RepoOwner, RepoName string
	BuildMeta           store.BuildMeta
	TS                  time.Time
}

func (c *MockBuildCreator) CreateBuild(
	ctx context.Context,
	installationID github.InstallationID,
	repoOwner, repoName string,
	build store.BuildMeta,
	ts time.Time,
) (uint64, error) {
	if c.build != nil {
		panic("Can only create one build per MockBuildCreator")
	}

	c.build = &MockBuild{
		InstallationID: installationID,
		RepoOwner:      repoOwner,
		RepoName:       repoName,
		BuildMeta:      build,
		TS:             ts,
	}

	return 1, nil
}

func validConfig() *config.Config {
	return &config.Config{
		GitHub: config.GitHubConfig{
			AuthorizedInstallations: []uint64{92907551},
			WebhookSecret:           testWebhookSecret,
		},
		Repos: []config.RepoConfig{{Owner: "ctbur", Name: "ci-server"}},
	}
}

func runWebhook(
	t *testing.T, cfg *config.Config, header http.Header, payload string,
) (*httptest.ResponseRecorder, *MockBuildCreator) {
	t.Helper()
	creator := &MockBuildCreator{}
	handler := HandleGitHub(creator, cfg)

	req := httptest.NewRequest(http.MethodPost, "/", io.NopCloser(strings.NewReader(payload)))
	req.Header = header

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr, creator
}

func TestGitHubWebhook_EventFiltering(t *testing.T) {
	tests := []struct {
		name        string
		event       string
		wantCode    int
		wantCreated bool
	}{
		{
			name:        "push event creates build",
			event:       "push",
			wantCode:    http.StatusOK,
			wantCreated: true,
		},
		{
			name:        "pull_request ignored",
			event:       "pull_request",
			wantCode:    http.StatusOK,
			wantCreated: false,
		},
		{
			name:        "issues ignored",
			event:       "issues",
			wantCode:    http.StatusOK,
			wantCreated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := withHeader(baseHeader, "X-Github-Event", tt.event)
			rr, creator := runWebhook(t, validConfig(), header, pushPayload)

			assert.Equal(t, rr.Code, tt.wantCode, "status code")
			if tt.wantCreated {
				if creator.build == nil {
					t.Error("expected build to be created")
				}
			} else {
				if creator.build != nil {
					t.Error("expected no build")
				}
			}
		})
	}
}

func TestGitHubWebhook_SignatureValidation(t *testing.T) {
	tests := []struct {
		name     string
		header   http.Header
		wantCode int
	}{
		{
			name:     "missing signature",
			header:   withoutHeader(baseHeader, "X-Hub-Signature-256"),
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "invalid signature",
			header:   withHeader(baseHeader, "X-Hub-Signature-256", "sha256=invalid"),
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "valid signature",
			header:   baseHeader,
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr, _ := runWebhook(t, validConfig(), tt.header, pushPayload)
			assert.Equal(t, rr.Code, tt.wantCode, "status code")
		})
	}
}

func TestGitHubWebhook_InstallationAuthorization(t *testing.T) {
	testCases := []struct {
		name            string
		installationIDs []uint64
		wantCode        int
	}{
		{
			name:            "no installations configured",
			installationIDs: []uint64{},
			wantCode:        http.StatusForbidden,
		},
		{
			name:            "wrong installation",
			installationIDs: []uint64{123456789},
			wantCode:        http.StatusForbidden,
		},
		{
			name:            "correct installation",
			installationIDs: []uint64{92907551},
			wantCode:        http.StatusOK,
		},
		{
			name:            "correct among multiple",
			installationIDs: []uint64{111, 92907551, 222},
			wantCode:        http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.GitHub.AuthorizedInstallations = tc.installationIDs

			rr, _ := runWebhook(t, cfg, baseHeader, pushPayload)
			assert.Equal(t, rr.Code, tc.wantCode, "status code")
		})
	}
}

func TestGitHubWebhook_ValidPush_CreatesBuild(t *testing.T) {
	rr, creator := runWebhook(t, validConfig(), baseHeader, pushPayload)

	assert.Equal(t, rr.Code, http.StatusOK, "status code")

	expectedBuild := store.BuildMeta{
		Link:      "https://github.com/ctbur/ci-server/commit/8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4",
		Ref:       "refs/heads/main",
		CommitSHA: "8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4",
		Message:   "Fix app token and improve dev setup (#21)",
		Author:    "ctbur",
	}
	assert.Equal(t, creator.build.BuildMeta, expectedBuild, "Incorrect build created")
}

func TestGitHubWebhook_BranchDeletion_NoBuild(t *testing.T) {
	header := withHeader(baseHeader, "X-Hub-Signature-256",
		"sha256=4f697a0572e015fe5e2a125ce6fde8f0005d1bdb7a710b0d1aab44a800835f9d")

	rr, creator := runWebhook(t, validConfig(), header, branchDeletePayload)

	assert.Equal(t, rr.Code, http.StatusOK, "status code")
	if creator.build != nil {
		t.Error("should not create build for branch deletion")
	}
}

func TestGitHubWebhook_UnconfiguredRepo_NotFound(t *testing.T) {
	cfg := validConfig()
	cfg.Repos = []config.RepoConfig{{Owner: "other", Name: "repo"}}

	rr, _ := runWebhook(t, cfg, baseHeader, pushPayload)

	assert.Equal(t, rr.Code, http.StatusNotFound, "status code")
}

func TestGitHubWebhook_MissingSecret_InternalError(t *testing.T) {
	cfg := validConfig()
	cfg.GitHub.WebhookSecret = ""

	rr, _ := runWebhook(t, cfg, baseHeader, pushPayload)

	assert.Equal(t, rr.Code, http.StatusInternalServerError, "status code")
}

func TestGitHubWebhook_InvalidCommitSHA_BadRequest(t *testing.T) {
	cfg := validConfig()
	payload := strings.ReplaceAll(
		pushPayload, "8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4", "not-a-sha",
	)
	header := withHeader(baseHeader, "X-Hub-Signature-256",
		"sha256=9b8295479494ed64b9a17dca7298c97f1effae328bd529d659484fcd1e6e247a")

	rr, _ := runWebhook(t, cfg, header, payload)

	assert.Equal(t, rr.Code, http.StatusBadRequest, "status code")
}
