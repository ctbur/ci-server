package webhook

const webhookSecret = "1234"

const headers = `Request URL: https://example.com/postreceive
Request method: POST
Accept: */*
Content-Type: application/json
User-Agent: GitHub-Hookshot/2444035
X-GitHub-Delivery: f2da33dc-9a3b-11f0-9aea-a663f22a3216
X-GitHub-Event: push
X-GitHub-Hook-ID: 571734419
X-GitHub-Hook-Installation-Target-ID: 926103085
X-GitHub-Hook-Installation-Target-Type: repository
X-Hub-Signature: sha1=59aca3565252348e38a3853095716a8009499d76
X-Hub-Signature-256: sha256=2accf62762a4e199493d9d8185ba9c1771b79a6cb555e84e228897da40260f08`

const payload = `{
  "ref": "refs/heads/test-branch",
  "before": "2cb63b571e10023a846ce9f46a00f7c3e70063a7",
  "after": "0000000000000000000000000000000000000000",
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
    "updated_at": "2025-09-25T14:00:11Z",
    "pushed_at": 1758824271,
    "git_url": "git://github.com/ctbur/ci-server.git",
    "ssh_url": "git@github.com:ctbur/ci-server.git",
    "clone_url": "https://github.com/ctbur/ci-server.git",
    "svn_url": "https://github.com/ctbur/ci-server",
    "homepage": null,
    "size": 75,
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
  "created": false,
  "deleted": true,
  "forced": false,
  "base_ref": null,
  "compare": "https://github.com/ctbur/ci-server/compare/2cb63b571e10...000000000000",
  "commits": [

  ],
  "head_commit": null
}`
