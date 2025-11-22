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
	"github.com/ctbur/ci-server/v2/internal/store"
)

var fixWebhookSecret = "sLBCQgxE29C1mgA0EVt4n2RMBPdH6iq1" // Not a valid secret, just for testing

var fixHeader http.Header = map[string][]string{
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

const fixPayload = `{"ref":"refs/heads/main","before":"26a2bf7cb1bf84c5d18a23ec132eab935386d6ab","after":"8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4","repository":{"id":926103085,"node_id":"R_kgDONzM2LQ","name":"ci-server","full_name":"ctbur/ci-server","private":false,"owner":{"name":"ctbur","email":"41328971+ctbur@users.noreply.github.com","login":"ctbur","id":41328971,"node_id":"MDQ6VXNlcjQxMzI4OTcx","avatar_url":"https://avatars.githubusercontent.com/u/41328971?v=4","gravatar_id":"","url":"https://api.github.com/users/ctbur","html_url":"https://github.com/ctbur","followers_url":"https://api.github.com/users/ctbur/followers","following_url":"https://api.github.com/users/ctbur/following{/other_user}","gists_url":"https://api.github.com/users/ctbur/gists{/gist_id}","starred_url":"https://api.github.com/users/ctbur/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/ctbur/subscriptions","organizations_url":"https://api.github.com/users/ctbur/orgs","repos_url":"https://api.github.com/users/ctbur/repos","events_url":"https://api.github.com/users/ctbur/events{/privacy}","received_events_url":"https://api.github.com/users/ctbur/received_events","type":"User","user_view_type":"public","site_admin":false},"html_url":"https://github.com/ctbur/ci-server","description":null,"fork":false,"url":"https://api.github.com/repos/ctbur/ci-server","forks_url":"https://api.github.com/repos/ctbur/ci-server/forks","keys_url":"https://api.github.com/repos/ctbur/ci-server/keys{/key_id}","collaborators_url":"https://api.github.com/repos/ctbur/ci-server/collaborators{/collaborator}","teams_url":"https://api.github.com/repos/ctbur/ci-server/teams","hooks_url":"https://api.github.com/repos/ctbur/ci-server/hooks","issue_events_url":"https://api.github.com/repos/ctbur/ci-server/issues/events{/number}","events_url":"https://api.github.com/repos/ctbur/ci-server/events","assignees_url":"https://api.github.com/repos/ctbur/ci-server/assignees{/user}","branches_url":"https://api.github.com/repos/ctbur/ci-server/branches{/branch}","tags_url":"https://api.github.com/repos/ctbur/ci-server/tags","blobs_url":"https://api.github.com/repos/ctbur/ci-server/git/blobs{/sha}","git_tags_url":"https://api.github.com/repos/ctbur/ci-server/git/tags{/sha}","git_refs_url":"https://api.github.com/repos/ctbur/ci-server/git/refs{/sha}","trees_url":"https://api.github.com/repos/ctbur/ci-server/git/trees{/sha}","statuses_url":"https://api.github.com/repos/ctbur/ci-server/statuses/{sha}","languages_url":"https://api.github.com/repos/ctbur/ci-server/languages","stargazers_url":"https://api.github.com/repos/ctbur/ci-server/stargazers","contributors_url":"https://api.github.com/repos/ctbur/ci-server/contributors","subscribers_url":"https://api.github.com/repos/ctbur/ci-server/subscribers","subscription_url":"https://api.github.com/repos/ctbur/ci-server/subscription","commits_url":"https://api.github.com/repos/ctbur/ci-server/commits{/sha}","git_commits_url":"https://api.github.com/repos/ctbur/ci-server/git/commits{/sha}","comments_url":"https://api.github.com/repos/ctbur/ci-server/comments{/number}","issue_comment_url":"https://api.github.com/repos/ctbur/ci-server/issues/comments{/number}","contents_url":"https://api.github.com/repos/ctbur/ci-server/contents/{+path}","compare_url":"https://api.github.com/repos/ctbur/ci-server/compare/{base}...{head}","merges_url":"https://api.github.com/repos/ctbur/ci-server/merges","archive_url":"https://api.github.com/repos/ctbur/ci-server/{archive_format}{/ref}","downloads_url":"https://api.github.com/repos/ctbur/ci-server/downloads","issues_url":"https://api.github.com/repos/ctbur/ci-server/issues{/number}","pulls_url":"https://api.github.com/repos/ctbur/ci-server/pulls{/number}","milestones_url":"https://api.github.com/repos/ctbur/ci-server/milestones{/number}","notifications_url":"https://api.github.com/repos/ctbur/ci-server/notifications{?since,all,participating}","labels_url":"https://api.github.com/repos/ctbur/ci-server/labels{/name}","releases_url":"https://api.github.com/repos/ctbur/ci-server/releases{/id}","deployments_url":"https://api.github.com/repos/ctbur/ci-server/deployments","created_at":1738508779,"updated_at":"2025-11-22T09:27:24Z","pushed_at":1763812047,"git_url":"git://github.com/ctbur/ci-server.git","ssh_url":"git@github.com:ctbur/ci-server.git","clone_url":"https://github.com/ctbur/ci-server.git","svn_url":"https://github.com/ctbur/ci-server","homepage":null,"size":209,"stargazers_count":0,"watchers_count":0,"language":"Go","has_issues":true,"has_projects":true,"has_downloads":true,"has_wiki":false,"has_pages":false,"has_discussions":false,"forks_count":0,"mirror_url":null,"archived":false,"disabled":false,"open_issues_count":0,"license":null,"allow_forking":true,"is_template":false,"web_commit_signoff_required":false,"topics":[],"visibility":"public","forks":0,"open_issues":0,"watchers":0,"default_branch":"main","stargazers":0,"master_branch":"main"},"pusher":{"name":"ctbur","email":"41328971+ctbur@users.noreply.github.com"},"sender":{"login":"ctbur","id":41328971,"node_id":"MDQ6VXNlcjQxMzI4OTcx","avatar_url":"https://avatars.githubusercontent.com/u/41328971?v=4","gravatar_id":"","url":"https://api.github.com/users/ctbur","html_url":"https://github.com/ctbur","followers_url":"https://api.github.com/users/ctbur/followers","following_url":"https://api.github.com/users/ctbur/following{/other_user}","gists_url":"https://api.github.com/users/ctbur/gists{/gist_id}","starred_url":"https://api.github.com/users/ctbur/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/ctbur/subscriptions","organizations_url":"https://api.github.com/users/ctbur/orgs","repos_url":"https://api.github.com/users/ctbur/repos","events_url":"https://api.github.com/users/ctbur/events{/privacy}","received_events_url":"https://api.github.com/users/ctbur/received_events","type":"User","user_view_type":"public","site_admin":false},"installation":{"id":92907551,"node_id":"MDIzOkludGVncmF0aW9uSW5zdGFsbGF0aW9uOTI5MDc1NTE="},"created":false,"deleted":false,"forced":false,"base_ref":null,"compare":"https://github.com/ctbur/ci-server/compare/26a2bf7cb1bf...8caeb971b6ff","commits":[{"id":"8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4","tree_id":"1b0be37edecd04276ab165d7920c27a404643082","distinct":true,"message":"Fix app token and improve dev setup (#21)","timestamp":"2025-11-22T12:47:27+01:00","url":"https://github.com/ctbur/ci-server/commit/8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4","author":{"name":"Cyrill Burgener","email":"41328971+ctbur@users.noreply.github.com","date":"2025-11-22T12:47:27+01:00","username":"ctbur"},"committer":{"name":"GitHub","email":"noreply@github.com","date":"2025-11-22T12:47:27+01:00","username":"web-flow"},"added":[".zed/debug.json"],"removed":["ci-config.toml","users.htpasswd"],"modified":[".gitignore","Makefile","cmd/server/main.go","internal/github/github.go","scripts/manual-webhook.sh"]}],"head_commit":{"id":"8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4","tree_id":"1b0be37edecd04276ab165d7920c27a404643082","distinct":true,"message":"Fix app token and improve dev setup (#21)","timestamp":"2025-11-22T12:47:27+01:00","url":"https://github.com/ctbur/ci-server/commit/8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4","author":{"name":"Cyrill Burgener","email":"41328971+ctbur@users.noreply.github.com","date":"2025-11-22T12:47:27+01:00","username":"ctbur"},"committer":{"name":"GitHub","email":"noreply@github.com","date":"2025-11-22T12:47:27+01:00","username":"web-flow"},"added":[".zed/debug.json"],"removed":["ci-config.toml","users.htpasswd"],"modified":[".gitignore","Makefile","cmd/server/main.go","internal/github/github.go","scripts/manual-webhook.sh"]}}`

const fixPayloadEmptyCommits = `{"ref":"refs/heads/fix-installation-token","before":"5209412abee89ed9774d7e3779a91964801d6790","after":"0000000000000000000000000000000000000000","repository":{"id":926103085,"node_id":"R_kgDONzM2LQ","name":"ci-server","full_name":"ctbur/ci-server","private":false,"owner":{"name":"ctbur","email":"41328971+ctbur@users.noreply.github.com","login":"ctbur","id":41328971,"node_id":"MDQ6VXNlcjQxMzI4OTcx","avatar_url":"https://avatars.githubusercontent.com/u/41328971?v=4","gravatar_id":"","url":"https://api.github.com/users/ctbur","html_url":"https://github.com/ctbur","followers_url":"https://api.github.com/users/ctbur/followers","following_url":"https://api.github.com/users/ctbur/following{/other_user}","gists_url":"https://api.github.com/users/ctbur/gists{/gist_id}","starred_url":"https://api.github.com/users/ctbur/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/ctbur/subscriptions","organizations_url":"https://api.github.com/users/ctbur/orgs","repos_url":"https://api.github.com/users/ctbur/repos","events_url":"https://api.github.com/users/ctbur/events{/privacy}","received_events_url":"https://api.github.com/users/ctbur/received_events","type":"User","user_view_type":"public","site_admin":false},"html_url":"https://github.com/ctbur/ci-server","description":null,"fork":false,"url":"https://api.github.com/repos/ctbur/ci-server","forks_url":"https://api.github.com/repos/ctbur/ci-server/forks","keys_url":"https://api.github.com/repos/ctbur/ci-server/keys{/key_id}","collaborators_url":"https://api.github.com/repos/ctbur/ci-server/collaborators{/collaborator}","teams_url":"https://api.github.com/repos/ctbur/ci-server/teams","hooks_url":"https://api.github.com/repos/ctbur/ci-server/hooks","issue_events_url":"https://api.github.com/repos/ctbur/ci-server/issues/events{/number}","events_url":"https://api.github.com/repos/ctbur/ci-server/events","assignees_url":"https://api.github.com/repos/ctbur/ci-server/assignees{/user}","branches_url":"https://api.github.com/repos/ctbur/ci-server/branches{/branch}","tags_url":"https://api.github.com/repos/ctbur/ci-server/tags","blobs_url":"https://api.github.com/repos/ctbur/ci-server/git/blobs{/sha}","git_tags_url":"https://api.github.com/repos/ctbur/ci-server/git/tags{/sha}","git_refs_url":"https://api.github.com/repos/ctbur/ci-server/git/refs{/sha}","trees_url":"https://api.github.com/repos/ctbur/ci-server/git/trees{/sha}","statuses_url":"https://api.github.com/repos/ctbur/ci-server/statuses/{sha}","languages_url":"https://api.github.com/repos/ctbur/ci-server/languages","stargazers_url":"https://api.github.com/repos/ctbur/ci-server/stargazers","contributors_url":"https://api.github.com/repos/ctbur/ci-server/contributors","subscribers_url":"https://api.github.com/repos/ctbur/ci-server/subscribers","subscription_url":"https://api.github.com/repos/ctbur/ci-server/subscription","commits_url":"https://api.github.com/repos/ctbur/ci-server/commits{/sha}","git_commits_url":"https://api.github.com/repos/ctbur/ci-server/git/commits{/sha}","comments_url":"https://api.github.com/repos/ctbur/ci-server/comments{/number}","issue_comment_url":"https://api.github.com/repos/ctbur/ci-server/issues/comments{/number}","contents_url":"https://api.github.com/repos/ctbur/ci-server/contents/{+path}","compare_url":"https://api.github.com/repos/ctbur/ci-server/compare/{base}...{head}","merges_url":"https://api.github.com/repos/ctbur/ci-server/merges","archive_url":"https://api.github.com/repos/ctbur/ci-server/{archive_format}{/ref}","downloads_url":"https://api.github.com/repos/ctbur/ci-server/downloads","issues_url":"https://api.github.com/repos/ctbur/ci-server/issues{/number}","pulls_url":"https://api.github.com/repos/ctbur/ci-server/pulls{/number}","milestones_url":"https://api.github.com/repos/ctbur/ci-server/milestones{/number}","notifications_url":"https://api.github.com/repos/ctbur/ci-server/notifications{?since,all,participating}","labels_url":"https://api.github.com/repos/ctbur/ci-server/labels{/name}","releases_url":"https://api.github.com/repos/ctbur/ci-server/releases{/id}","deployments_url":"https://api.github.com/repos/ctbur/ci-server/deployments","created_at":1738508779,"updated_at":"2025-11-22T09:27:24Z","pushed_at":1763812048,"git_url":"git://github.com/ctbur/ci-server.git","ssh_url":"git@github.com:ctbur/ci-server.git","clone_url":"https://github.com/ctbur/ci-server.git","svn_url":"https://github.com/ctbur/ci-server","homepage":null,"size":209,"stargazers_count":0,"watchers_count":0,"language":"Go","has_issues":true,"has_projects":true,"has_downloads":true,"has_wiki":false,"has_pages":false,"has_discussions":false,"forks_count":0,"mirror_url":null,"archived":false,"disabled":false,"open_issues_count":0,"license":null,"allow_forking":true,"is_template":false,"web_commit_signoff_required":false,"topics":[],"visibility":"public","forks":0,"open_issues":0,"watchers":0,"default_branch":"main","stargazers":0,"master_branch":"main"},"pusher":{"name":"ctbur","email":"41328971+ctbur@users.noreply.github.com"},"sender":{"login":"ctbur","id":41328971,"node_id":"MDQ6VXNlcjQxMzI4OTcx","avatar_url":"https://avatars.githubusercontent.com/u/41328971?v=4","gravatar_id":"","url":"https://api.github.com/users/ctbur","html_url":"https://github.com/ctbur","followers_url":"https://api.github.com/users/ctbur/followers","following_url":"https://api.github.com/users/ctbur/following{/other_user}","gists_url":"https://api.github.com/users/ctbur/gists{/gist_id}","starred_url":"https://api.github.com/users/ctbur/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/ctbur/subscriptions","organizations_url":"https://api.github.com/users/ctbur/orgs","repos_url":"https://api.github.com/users/ctbur/repos","events_url":"https://api.github.com/users/ctbur/events{/privacy}","received_events_url":"https://api.github.com/users/ctbur/received_events","type":"User","user_view_type":"public","site_admin":false},"installation":{"id":92907551,"node_id":"MDIzOkludGVncmF0aW9uSW5zdGFsbGF0aW9uOTI5MDc1NTE="},"created":false,"deleted":true,"forced":false,"base_ref":null,"compare":"https://github.com/ctbur/ci-server/compare/5209412abee8...000000000000","commits":[],"head_commit":null}`

func headerSet(header http.Header, key, value string) http.Header {
	h := header.Clone()
	h.Set(key, value)
	return h
}

func headerDel(header http.Header, key string) http.Header {
	h := header.Clone()
	h.Del(key)
	return h
}

type MockBuildCreator struct {
	Build *MockBuild
}

type MockBuild struct {
	RepoOwner, RepoName string
	BuildMeta           store.BuildMeta
	TS                  time.Time
}

func (c *MockBuildCreator) CreateBuild(
	ctx context.Context, repoOwner, repoName string, build store.BuildMeta, ts time.Time,
) (uint64, error) {
	if c.Build != nil {
		panic("Can only create one build per MockBuildCreator")
	}

	c.Build = &MockBuild{
		RepoOwner: repoOwner,
		RepoName:  repoName,
		BuildMeta: build,
		TS:        ts,
	}

	return 1, nil
}

func TestGitHubWebhook(t *testing.T) {
	validBuild := store.BuildMeta{
		Link:      "https://github.com/ctbur/ci-server/commit/8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4",
		Ref:       "refs/heads/main",
		CommitSHA: "8caeb971b6ffa0e1d3049baeefc17226ebc4e6d4",
		Message:   "Fix app token and improve dev setup (#21)",
		Author:    "ctbur",
	}

	testCases := []struct {
		desc                      string
		header                    http.Header
		payload                   string
		repoOwner                 string
		repoName                  string
		authorizedInstallationIDs []uint64
		webhookSecret             string
		wantHTTPCode              int
		wantBuild                 *store.BuildMeta
	}{
		{
			desc:                      "unrelated GitHub event",
			header:                    headerSet(fixHeader, "X-GitHub-Event", "pull_request"),
			payload:                   fixPayload,
			repoOwner:                 "ctbur",
			repoName:                  "ci-server",
			authorizedInstallationIDs: []uint64{92907551},
			webhookSecret:             fixWebhookSecret,
			wantHTTPCode:              http.StatusOK,
			wantBuild:                 nil,
		},
		{
			desc:                      "invalid commit SHA",
			header:                    headerSet(fixHeader, "X-Hub-Signature-256", "sha256=712a5ca4746defb51d6e500af11dfe38062b10595aa8d9612b198afa6545d770"),
			payload:                   strings.ReplaceAll(fixPayload, validBuild.CommitSHA, "not a commit SHA"),
			repoOwner:                 "ctbur",
			repoName:                  "ci-server",
			authorizedInstallationIDs: []uint64{92907551},
			webhookSecret:             fixWebhookSecret,
			wantHTTPCode:              http.StatusBadRequest,
			wantBuild:                 nil,
		},
		{
			desc:                      "missing signature",
			header:                    headerDel(fixHeader, "X-Hub-Signature-256"),
			payload:                   fixPayload,
			repoOwner:                 "ctbur",
			repoName:                  "ci-server",
			authorizedInstallationIDs: []uint64{92907551},
			webhookSecret:             fixWebhookSecret,
			wantHTTPCode:              http.StatusUnauthorized,
			wantBuild:                 nil,
		},
		{
			desc:                      "incorrect signature",
			header:                    headerSet(fixHeader, "X-Hub-Signature-256", "sha256=120e0601c13771b59ff8e5f968619fea6eb0827d47e6082080b6bea6e37b6227"),
			payload:                   fixPayload,
			repoOwner:                 "ctbur",
			repoName:                  "ci-server",
			authorizedInstallationIDs: []uint64{92907551},
			webhookSecret:             fixWebhookSecret,
			wantHTTPCode:              http.StatusUnauthorized,
			wantBuild:                 nil,
		},
		{
			desc:                      "repository not configured",
			header:                    fixHeader,
			payload:                   fixPayload,
			repoOwner:                 "ctbur",
			repoName:                  "other-repo",
			authorizedInstallationIDs: []uint64{92907551},
			webhookSecret:             fixWebhookSecret,
			wantHTTPCode:              http.StatusNotFound,
			wantBuild:                 nil,
		},
		{
			desc:                      "webhookSecret not configured",
			header:                    fixHeader,
			payload:                   fixPayload,
			repoOwner:                 "ctbur",
			repoName:                  "ci-server",
			authorizedInstallationIDs: []uint64{92907551},
			webhookSecret:             "",
			wantHTTPCode:              http.StatusInternalServerError,
			wantBuild:                 nil,
		},
		{
			desc:                      "correct configuration",
			header:                    fixHeader,
			payload:                   fixPayload,
			repoOwner:                 "ctbur",
			repoName:                  "ci-server",
			authorizedInstallationIDs: []uint64{92907551},
			webhookSecret:             fixWebhookSecret,
			wantHTTPCode:              http.StatusOK,
			wantBuild:                 &validBuild,
		},
		{
			desc:                      "branch deletion",
			header:                    headerSet(fixHeader, "X-Hub-Signature-256", "sha256=4f697a0572e015fe5e2a125ce6fde8f0005d1bdb7a710b0d1aab44a800835f9d"),
			payload:                   fixPayloadEmptyCommits,
			repoOwner:                 "ctbur",
			repoName:                  "ci-server",
			authorizedInstallationIDs: []uint64{92907551},
			webhookSecret:             fixWebhookSecret,
			wantHTTPCode:              http.StatusOK,
			wantBuild:                 nil, // No build created on branch deletion
		},
		{
			desc:                      "no authorized installations configured",
			header:                    fixHeader,
			payload:                   fixPayload,
			repoOwner:                 "ctbur",
			repoName:                  "ci-server",
			authorizedInstallationIDs: []uint64{},
			webhookSecret:             fixWebhookSecret,
			wantHTTPCode:              http.StatusForbidden,
			wantBuild:                 nil,
		},
		{
			desc:                      "multiple authorized installations, correct one",
			header:                    fixHeader,
			payload:                   fixPayload,
			repoOwner:                 "ctbur",
			repoName:                  "ci-server",
			authorizedInstallationIDs: []uint64{111111111, 92907551, 222222222},
			webhookSecret:             fixWebhookSecret,
			wantHTTPCode:              http.StatusOK,
			wantBuild:                 &validBuild,
		},
		{
			desc:                      "unauthorized installation",
			header:                    fixHeader,
			payload:                   fixPayload,
			repoOwner:                 "ctbur",
			repoName:                  "ci-server",
			authorizedInstallationIDs: []uint64{123456789}, // Different installation ID
			webhookSecret:             fixWebhookSecret,
			wantHTTPCode:              http.StatusForbidden,
			wantBuild:                 nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Given
			cfg := config.Config{
				GitHub: &config.GitHubConfig{
					AuthorizedInstallationIDs: tc.authorizedInstallationIDs,
					WebhookSecret:             tc.webhookSecret,
				},
				Repos: []config.RepoConfig{
					{
						Owner: tc.repoOwner,
						Name:  tc.repoName,
					},
				},
			}

			c := MockBuildCreator{}

			// When
			webhook := http.Handler(HandleGitHub(&c, &cfg))

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header = tc.header
			req.Body = io.NopCloser(strings.NewReader(tc.payload))

			rr := httptest.NewRecorder()
			webhook.ServeHTTP(rr, req)

			// Then
			assert.Equal(t, rr.Code, tc.wantHTTPCode, "handler returned wrong status code")

			if tc.wantBuild != nil {
				if c.Build != nil {
					assert.Equal(t, c.Build.BuildMeta, *tc.wantBuild, "Incorrect build created")
				} else {
					t.Error("Build was not created when it should have been")
				}
			} else {
				assert.Equal(t, c.Build, nil, "Build was created mistakenly")
			}
		})
	}
}
