package webhook

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ctbur/ci-server/v2/internal/assert"
	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
)

var fixHeader http.Header = map[string][]string{
	"Accept":                                 {"*/*"},
	"Content-Type":                           {"application/json"},
	"User-Agent":                             {"GitHub-Hookshot/2444035"},
	"X-Github-Delivery":                      {"c84faa40-9b83-11f0-99ab-b556eeccde17"},
	"X-Github-Event":                         {"push"},
	"X-Github-Hook-ID":                       {"572032031"},
	"X-Github-Hook-Installation-Target-ID":   {"926103085"},
	"X-Github-Hook-Installation-Target-Type": {"repository"},
	"X-Hub-Signature":                        {"sha1=4553de0f8be021e5a75f744ef7539c196214207b"},
	"X-Hub-Signature-256":                    {"sha256=020e0601c13771b59ff8e5f968619fea6eb0827d47e6082080b6bea6e37b6227"},
}

const fixPayload = `{
  "ref": "refs/heads/test-branch",
  "before": "0000000000000000000000000000000000000000",
  "after": "efe2dcb5a7888db60449e66dd1558b971b4f54d5",
  "repository": {
    "id": 926103085,
    "node_id": "R_kgDONzM2LQ",
    "name": "ci-server",
    "full_name": "ctbur/ci-server",
    "private": true,
    "owner": {
      "name": "ctbur",
      "email": "41328971+ctbur@users.noreply.github.com",
      "login": "ctbur",
      "id": 41328971,
      "node_id": "MDQ6VXNlcjQxMzI4OTcx",
      "avatar_url": "https://avatars.githubusercontent.com/u/41328971?v=4",
      "gravatar_id": "",
      "url": "https://api.github.com/users/ctbur",
      "html_url": "https://github.com/ctbur",
      "followers_url": "https://api.github.com/users/ctbur/followers",
      "following_url": "https://api.github.com/users/ctbur/following{/other_user}",
      "gists_url": "https://api.github.com/users/ctbur/gists{/gist_id}",
      "starred_url": "https://api.github.com/users/ctbur/starred{/owner}{/repo}",
      "subscriptions_url": "https://api.github.com/users/ctbur/subscriptions",
      "organizations_url": "https://api.github.com/users/ctbur/orgs",
      "repos_url": "https://api.github.com/users/ctbur/repos",
      "events_url": "https://api.github.com/users/ctbur/events{/privacy}",
      "received_events_url": "https://api.github.com/users/ctbur/received_events",
      "type": "User",
      "user_view_type": "public",
      "site_admin": false
    },
    "html_url": "https://github.com/ctbur/ci-server",
    "description": null,
    "fork": false,
    "url": "https://api.github.com/repos/ctbur/ci-server",
    "forks_url": "https://api.github.com/repos/ctbur/ci-server/forks",
    "keys_url": "https://api.github.com/repos/ctbur/ci-server/keys{/key_id}",
    "collaborators_url": "https://api.github.com/repos/ctbur/ci-server/collaborators{/collaborator}",
    "teams_url": "https://api.github.com/repos/ctbur/ci-server/teams",
    "hooks_url": "https://api.github.com/repos/ctbur/ci-server/hooks",
    "issue_events_url": "https://api.github.com/repos/ctbur/ci-server/issues/events{/number}",
    "events_url": "https://api.github.com/repos/ctbur/ci-server/events",
    "assignees_url": "https://api.github.com/repos/ctbur/ci-server/assignees{/user}",
    "branches_url": "https://api.github.com/repos/ctbur/ci-server/branches{/branch}",
    "tags_url": "https://api.github.com/repos/ctbur/ci-server/tags",
    "blobs_url": "https://api.github.com/repos/ctbur/ci-server/git/blobs{/sha}",
    "git_tags_url": "https://api.github.com/repos/ctbur/ci-server/git/tags{/sha}",
    "git_refs_url": "https://api.github.com/repos/ctbur/ci-server/git/refs{/sha}",
    "trees_url": "https://api.github.com/repos/ctbur/ci-server/git/trees{/sha}",
    "statuses_url": "https://api.github.com/repos/ctbur/ci-server/statuses/{sha}",
    "languages_url": "https://api.github.com/repos/ctbur/ci-server/languages",
    "stargazers_url": "https://api.github.com/repos/ctbur/ci-server/stargazers",
    "contributors_url": "https://api.github.com/repos/ctbur/ci-server/contributors",
    "subscribers_url": "https://api.github.com/repos/ctbur/ci-server/subscribers",
    "subscription_url": "https://api.github.com/repos/ctbur/ci-server/subscription",
    "commits_url": "https://api.github.com/repos/ctbur/ci-server/commits{/sha}",
    "git_commits_url": "https://api.github.com/repos/ctbur/ci-server/git/commits{/sha}",
    "comments_url": "https://api.github.com/repos/ctbur/ci-server/comments{/number}",
    "issue_comment_url": "https://api.github.com/repos/ctbur/ci-server/issues/comments{/number}",
    "contents_url": "https://api.github.com/repos/ctbur/ci-server/contents/{+path}",
    "compare_url": "https://api.github.com/repos/ctbur/ci-server/compare/{base}...{head}",
    "merges_url": "https://api.github.com/repos/ctbur/ci-server/merges",
    "archive_url": "https://api.github.com/repos/ctbur/ci-server/{archive_format}{/ref}",
    "downloads_url": "https://api.github.com/repos/ctbur/ci-server/downloads",
    "issues_url": "https://api.github.com/repos/ctbur/ci-server/issues{/number}",
    "pulls_url": "https://api.github.com/repos/ctbur/ci-server/pulls{/number}",
    "milestones_url": "https://api.github.com/repos/ctbur/ci-server/milestones{/number}",
    "notifications_url": "https://api.github.com/repos/ctbur/ci-server/notifications{?since,all,participating}",
    "labels_url": "https://api.github.com/repos/ctbur/ci-server/labels{/name}",
    "releases_url": "https://api.github.com/repos/ctbur/ci-server/releases{/id}",
    "deployments_url": "https://api.github.com/repos/ctbur/ci-server/deployments",
    "created_at": 1738508779,
    "updated_at": "2025-09-26T21:01:03Z",
    "pushed_at": 1758965075,
    "git_url": "git://github.com/ctbur/ci-server.git",
    "ssh_url": "git@github.com:ctbur/ci-server.git",
    "clone_url": "https://github.com/ctbur/ci-server.git",
    "svn_url": "https://github.com/ctbur/ci-server",
    "homepage": null,
    "size": 102,
    "stargazers_count": 0,
    "watchers_count": 0,
    "language": "Go",
    "has_issues": true,
    "has_projects": true,
    "has_downloads": true,
    "has_wiki": false,
    "has_pages": false,
    "has_discussions": false,
    "forks_count": 0,
    "mirror_url": null,
    "archived": false,
    "disabled": false,
    "open_issues_count": 0,
    "license": null,
    "allow_forking": true,
    "is_template": false,
    "web_commit_signoff_required": false,
    "topics": [

    ],
    "visibility": "private",
    "forks": 0,
    "open_issues": 0,
    "watchers": 0,
    "default_branch": "main",
    "stargazers": 0,
    "master_branch": "main"
  },
  "pusher": {
    "name": "ctbur",
    "email": "41328971+ctbur@users.noreply.github.com"
  },
  "sender": {
    "login": "ctbur",
    "id": 41328971,
    "node_id": "MDQ6VXNlcjQxMzI4OTcx",
    "avatar_url": "https://avatars.githubusercontent.com/u/41328971?v=4",
    "gravatar_id": "",
    "url": "https://api.github.com/users/ctbur",
    "html_url": "https://github.com/ctbur",
    "followers_url": "https://api.github.com/users/ctbur/followers",
    "following_url": "https://api.github.com/users/ctbur/following{/other_user}",
    "gists_url": "https://api.github.com/users/ctbur/gists{/gist_id}",
    "starred_url": "https://api.github.com/users/ctbur/starred{/owner}{/repo}",
    "subscriptions_url": "https://api.github.com/users/ctbur/subscriptions",
    "organizations_url": "https://api.github.com/users/ctbur/orgs",
    "repos_url": "https://api.github.com/users/ctbur/repos",
    "events_url": "https://api.github.com/users/ctbur/events{/privacy}",
    "received_events_url": "https://api.github.com/users/ctbur/received_events",
    "type": "User",
    "user_view_type": "public",
    "site_admin": false
  },
  "created": true,
  "deleted": false,
  "forced": false,
  "base_ref": null,
  "compare": "https://github.com/ctbur/ci-server/commit/efe2dcb5a788",
  "commits": [
    {
      "id": "efe2dcb5a7888db60449e66dd1558b971b4f54d5",
      "tree_id": "0aebe90d706957df4960f50f99d3b2f103d3f984",
      "distinct": true,
      "message": "Make a test commit",
      "timestamp": "2025-09-27T11:24:34+02:00",
      "url": "https://github.com/ctbur/ci-server/commit/efe2dcb5a7888db60449e66dd1558b971b4f54d5",
      "author": {
        "name": "Cyrill Burgener",
        "email": "41328971+ctbur@users.noreply.github.com",
        "username": "ctbur"
      },
      "committer": {
        "name": "GitHub",
        "email": "noreply@github.com",
        "username": "web-flow"
      },
      "added": [
        "README.md"
      ],
      "removed": [

      ],
      "modified": [

      ]
    }
  ],
  "head_commit": {
    "id": "efe2dcb5a7888db60449e66dd1558b971b4f54d5",
    "tree_id": "0aebe90d706957df4960f50f99d3b2f103d3f984",
    "distinct": true,
    "message": "Make a test commit",
    "timestamp": "2025-09-27T11:24:34+02:00",
    "url": "https://github.com/ctbur/ci-server/commit/efe2dcb5a7888db60449e66dd1558b971b4f54d5",
    "author": {
      "name": "Cyrill Burgener",
      "email": "41328971+ctbur@users.noreply.github.com",
      "username": "ctbur"
    },
    "committer": {
      "name": "GitHub",
      "email": "noreply@github.com",
      "username": "web-flow"
    },
    "added": [
      "README.md"
    ],
    "removed": [

    ],
    "modified": [

    ]
  }
}`

var fixWebhookSecret = "1234"

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

func CanonicalizePayload(formattedJSON string) string {
	// 1. Unmarshal into a generic map. This step strips all non-essential whitespace.
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(formattedJSON), &payload); err != nil {
		// Log the error and fail if the input JSON is structurally invalid
		log.Fatalf("CanonicalizePayload failed to unmarshal input: %v", err)
	}

	// 2. Marshal the map back into bytes.
	// The standard json.Marshal function guarantees the output is compressed (no indents or newlines).
	compressedBytes, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("CanonicalizePayload failed to marshal back to compressed JSON: %v", err)
	}

	return string(compressedBytes)
}

func TestGitHubWebhook(t *testing.T) {
	// test cases:
	// - build when repo configured and signature correct

	testCases := []struct {
		desc          string
		header        http.Header
		payload       string
		repoOwner     string
		repoName      string
		webhookSecret *string
		wantHTTPCode  int
		wantBuild     *store.BuildMeta
	}{
		{
			desc:          "unrelated GitHub event",
			header:        headerSet(fixHeader, "X-GitHub-Event", "pull_request"),
			payload:       fixPayload,
			repoOwner:     "ctbur",
			repoName:      "ci-server",
			webhookSecret: &fixWebhookSecret,
			wantHTTPCode:  http.StatusOK,
			wantBuild:     nil,
		},
		{
			desc:          "missing signature",
			header:        headerDel(fixHeader, "X-Hub-Signature-256"),
			payload:       fixPayload,
			repoOwner:     "ctbur",
			repoName:      "ci-server",
			webhookSecret: &fixWebhookSecret,
			wantHTTPCode:  http.StatusUnauthorized,
			wantBuild:     nil,
		},
		{
			desc:          "incorrect signature",
			header:        headerSet(fixHeader, "X-Hub-Signature-256", "sha256=120e0601c13771b59ff8e5f968619fea6eb0827d47e6082080b6bea6e37b6227"),
			payload:       fixPayload,
			repoOwner:     "ctbur",
			repoName:      "ci-server",
			webhookSecret: &fixWebhookSecret,
			wantHTTPCode:  http.StatusUnauthorized,
			wantBuild:     nil,
		},
		{
			desc:          "repository not configured",
			header:        fixHeader,
			payload:       fixPayload,
			repoOwner:     "ctbur",
			repoName:      "other-repo",
			webhookSecret: &fixWebhookSecret,
			wantHTTPCode:  http.StatusNotFound,
			wantBuild:     nil,
		},
		{
			desc:          "webhookSecret not configured",
			header:        fixHeader,
			payload:       fixPayload,
			repoOwner:     "ctbur",
			repoName:      "ci-server",
			webhookSecret: nil,
			wantHTTPCode:  http.StatusInternalServerError,
			wantBuild:     nil,
		},
		{
			desc:          "correct configuration",
			header:        fixHeader,
			payload:       fixPayload,
			repoOwner:     "ctbur",
			repoName:      "ci-server",
			webhookSecret: &fixWebhookSecret,
			wantHTTPCode:  http.StatusOK,
			wantBuild: &store.BuildMeta{
				Link:      "https://github.com/ctbur/ci-server/commit/efe2dcb5a7888db60449e66dd1558b971b4f54d5",
				Ref:       "refs/heads/test-branch",
				CommitSHA: "efe2dcb5a7888db60449e66dd1558b971b4f54d5",
				Message:   "Make a test commit",
				Author:    "ctbur",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Given
			cfg := config.Config{
				Repos: []config.RepoConfig{
					{
						Owner:         tc.repoOwner,
						Name:          tc.repoName,
						WebhookSecret: tc.webhookSecret,
					},
				},
			}

			s := store.NewMockBuildStore()
			ctx := context.Background()

			s.CreateRepoIfNotExists(ctx, store.RepoMeta{
				Owner: tc.repoOwner,
				Name:  tc.repoName,
			})

			// When
			webhook := http.Handler(handleGitHub(s, &cfg))

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header = tc.header
			req.Body = io.NopCloser(strings.NewReader(tc.payload))

			rr := httptest.NewRecorder()
			webhook.ServeHTTP(rr, req)

			// Then
			assert.Equal(t, rr.Code, tc.wantHTTPCode, "handler returned wrong status code")

			gotBuild, err := s.GetBuild(ctx, 1)
			if tc.wantBuild != nil {
				assert.NoError(t, err, "Error when fetching build")
				if gotBuild != nil {
					assert.Equal(t, gotBuild.BuildMeta, *tc.wantBuild, "Incorrect build created")
				}
			} else {
				assert.ErrorIs(t, err, store.ErrNoBuild, "Error or incorrect build creation")
			}
		})
	}
}
