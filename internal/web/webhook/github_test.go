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
	"User-Agent":                             {"GitHub-Hookshot/2444035"},
	"X-Github-Delivery":                      {"0c8806aa-9d30-11f0-876e-2f6051f733a7"},
	"X-Github-Event":                         {"push"},
	"X-Github-Hook-ID":                       {"572389829"},
	"X-Github-Hook-Installation-Target-ID":   {"758176159"},
	"X-Github-Hook-Installation-Target-Type": {"repository"},
	"X-Hub-Signature":                        {"sha1=4786d04e00b7e096196f1aea84542e08295e8c7c"},
	"X-Hub-Signature-256":                    {"sha256=58184ed0308095db0c9a28d70722aabf748bca18a843d4733baf9ca248a0aeac"},
}

const fixPayload = "{\"ref\":\"refs/heads/main\",\"before\":\"cc1d5656b1c6b6a0f6964f741e1c2d2c692fa886\",\"after\":\"c5ec2129a2a892156c8c97220e6059b9d47b7217\",\"repository\":{\"id\":758176159,\"node_id\":\"R_kgDOLTDZnw\",\"name\":\"ctbur.net\",\"full_name\":\"ctbur/ctbur.net\",\"private\":false,\"owner\":{\"name\":\"ctbur\",\"email\":\"41328971+ctbur@users.noreply.github.com\",\"login\":\"ctbur\",\"id\":41328971,\"node_id\":\"MDQ6VXNlcjQxMzI4OTcx\",\"avatar_url\":\"https://avatars.githubusercontent.com/u/41328971?v=4\",\"gravatar_id\":\"\",\"url\":\"https://api.github.com/users/ctbur\",\"html_url\":\"https://github.com/ctbur\",\"followers_url\":\"https://api.github.com/users/ctbur/followers\",\"following_url\":\"https://api.github.com/users/ctbur/following{/other_user}\",\"gists_url\":\"https://api.github.com/users/ctbur/gists{/gist_id}\",\"starred_url\":\"https://api.github.com/users/ctbur/starred{/owner}{/repo}\",\"subscriptions_url\":\"https://api.github.com/users/ctbur/subscriptions\",\"organizations_url\":\"https://api.github.com/users/ctbur/orgs\",\"repos_url\":\"https://api.github.com/users/ctbur/repos\",\"events_url\":\"https://api.github.com/users/ctbur/events{/privacy}\",\"received_events_url\":\"https://api.github.com/users/ctbur/received_events\",\"type\":\"User\",\"user_view_type\":\"public\",\"site_admin\":false},\"html_url\":\"https://github.com/ctbur/ctbur.net\",\"description\":\"My personal website\",\"fork\":false,\"url\":\"https://api.github.com/repos/ctbur/ctbur.net\",\"forks_url\":\"https://api.github.com/repos/ctbur/ctbur.net/forks\",\"keys_url\":\"https://api.github.com/repos/ctbur/ctbur.net/keys{/key_id}\",\"collaborators_url\":\"https://api.github.com/repos/ctbur/ctbur.net/collaborators{/collaborator}\",\"teams_url\":\"https://api.github.com/repos/ctbur/ctbur.net/teams\",\"hooks_url\":\"https://api.github.com/repos/ctbur/ctbur.net/hooks\",\"issue_events_url\":\"https://api.github.com/repos/ctbur/ctbur.net/issues/events{/number}\",\"events_url\":\"https://api.github.com/repos/ctbur/ctbur.net/events\",\"assignees_url\":\"https://api.github.com/repos/ctbur/ctbur.net/assignees{/user}\",\"branches_url\":\"https://api.github.com/repos/ctbur/ctbur.net/branches{/branch}\",\"tags_url\":\"https://api.github.com/repos/ctbur/ctbur.net/tags\",\"blobs_url\":\"https://api.github.com/repos/ctbur/ctbur.net/git/blobs{/sha}\",\"git_tags_url\":\"https://api.github.com/repos/ctbur/ctbur.net/git/tags{/sha}\",\"git_refs_url\":\"https://api.github.com/repos/ctbur/ctbur.net/git/refs{/sha}\",\"trees_url\":\"https://api.github.com/repos/ctbur/ctbur.net/git/trees{/sha}\",\"statuses_url\":\"https://api.github.com/repos/ctbur/ctbur.net/statuses/{sha}\",\"languages_url\":\"https://api.github.com/repos/ctbur/ctbur.net/languages\",\"stargazers_url\":\"https://api.github.com/repos/ctbur/ctbur.net/stargazers\",\"contributors_url\":\"https://api.github.com/repos/ctbur/ctbur.net/contributors\",\"subscribers_url\":\"https://api.github.com/repos/ctbur/ctbur.net/subscribers\",\"subscription_url\":\"https://api.github.com/repos/ctbur/ctbur.net/subscription\",\"commits_url\":\"https://api.github.com/repos/ctbur/ctbur.net/commits{/sha}\",\"git_commits_url\":\"https://api.github.com/repos/ctbur/ctbur.net/git/commits{/sha}\",\"comments_url\":\"https://api.github.com/repos/ctbur/ctbur.net/comments{/number}\",\"issue_comment_url\":\"https://api.github.com/repos/ctbur/ctbur.net/issues/comments{/number}\",\"contents_url\":\"https://api.github.com/repos/ctbur/ctbur.net/contents/{+path}\",\"compare_url\":\"https://api.github.com/repos/ctbur/ctbur.net/compare/{base}...{head}\",\"merges_url\":\"https://api.github.com/repos/ctbur/ctbur.net/merges\",\"archive_url\":\"https://api.github.com/repos/ctbur/ctbur.net/{archive_format}{/ref}\",\"downloads_url\":\"https://api.github.com/repos/ctbur/ctbur.net/downloads\",\"issues_url\":\"https://api.github.com/repos/ctbur/ctbur.net/issues{/number}\",\"pulls_url\":\"https://api.github.com/repos/ctbur/ctbur.net/pulls{/number}\",\"milestones_url\":\"https://api.github.com/repos/ctbur/ctbur.net/milestones{/number}\",\"notifications_url\":\"https://api.github.com/repos/ctbur/ctbur.net/notifications{?since,all,participating}\",\"labels_url\":\"https://api.github.com/repos/ctbur/ctbur.net/labels{/name}\",\"releases_url\":\"https://api.github.com/repos/ctbur/ctbur.net/releases{/id}\",\"deployments_url\":\"https://api.github.com/repos/ctbur/ctbur.net/deployments\",\"created_at\":1708024625,\"updated_at\":\"2025-08-12T12:09:15Z\",\"pushed_at\":1759149013,\"git_url\":\"git://github.com/ctbur/ctbur.net.git\",\"ssh_url\":\"git@github.com:ctbur/ctbur.net.git\",\"clone_url\":\"https://github.com/ctbur/ctbur.net.git\",\"svn_url\":\"https://github.com/ctbur/ctbur.net\",\"homepage\":null,\"size\":105,\"stargazers_count\":0,\"watchers_count\":0,\"language\":\"Go\",\"has_issues\":true,\"has_projects\":true,\"has_downloads\":true,\"has_wiki\":false,\"has_pages\":false,\"has_discussions\":false,\"forks_count\":1,\"mirror_url\":null,\"archived\":false,\"disabled\":false,\"open_issues_count\":5,\"license\":null,\"allow_forking\":true,\"is_template\":false,\"web_commit_signoff_required\":false,\"topics\":[],\"visibility\":\"public\",\"forks\":1,\"open_issues\":5,\"watchers\":0,\"default_branch\":\"main\",\"stargazers\":0,\"master_branch\":\"main\"},\"pusher\":{\"name\":\"ctbur\",\"email\":\"41328971+ctbur@users.noreply.github.com\"},\"sender\":{\"login\":\"ctbur\",\"id\":41328971,\"node_id\":\"MDQ6VXNlcjQxMzI4OTcx\",\"avatar_url\":\"https://avatars.githubusercontent.com/u/41328971?v=4\",\"gravatar_id\":\"\",\"url\":\"https://api.github.com/users/ctbur\",\"html_url\":\"https://github.com/ctbur\",\"followers_url\":\"https://api.github.com/users/ctbur/followers\",\"following_url\":\"https://api.github.com/users/ctbur/following{/other_user}\",\"gists_url\":\"https://api.github.com/users/ctbur/gists{/gist_id}\",\"starred_url\":\"https://api.github.com/users/ctbur/starred{/owner}{/repo}\",\"subscriptions_url\":\"https://api.github.com/users/ctbur/subscriptions\",\"organizations_url\":\"https://api.github.com/users/ctbur/orgs\",\"repos_url\":\"https://api.github.com/users/ctbur/repos\",\"events_url\":\"https://api.github.com/users/ctbur/events{/privacy}\",\"received_events_url\":\"https://api.github.com/users/ctbur/received_events\",\"type\":\"User\",\"user_view_type\":\"public\",\"site_admin\":false},\"created\":false,\"deleted\":false,\"forced\":false,\"base_ref\":null,\"compare\":\"https://github.com/ctbur/ctbur.net/compare/cc1d5656b1c6...c5ec2129a2a8\",\"commits\":[{\"id\":\"c5ec2129a2a892156c8c97220e6059b9d47b7217\",\"tree_id\":\"2b50039faff054f37ce836e6d491db9912e38779\",\"distinct\":true,\"message\":\"Bump actions/setup-go (#13)\\n\\nBumps [actions/setup-go](https://github.com/actions/setup-go) from 8e57b58e57be52ac95949151e2777ffda8501267 to c0137caad775660c0844396c52da96e560aba63d.\\n- [Release notes](https://github.com/actions/setup-go/releases)\\n- [Commits](https://github.com/actions/setup-go/compare/8e57b58e57be52ac95949151e2777ffda8501267...c0137caad775660c0844396c52da96e560aba63d)\\n\\n---\\nupdated-dependencies:\\n- dependency-name: actions/setup-go\\n  dependency-version: c0137caad775660c0844396c52da96e560aba63d\\n  dependency-type: direct:production\\n...\\n\\nSigned-off-by: dependabot[bot] <support@github.com>\\nCo-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>\",\"timestamp\":\"2025-09-29T14:30:13+02:00\",\"url\":\"https://github.com/ctbur/ctbur.net/commit/c5ec2129a2a892156c8c97220e6059b9d47b7217\",\"author\":{\"name\":\"dependabot[bot]\",\"email\":\"49699333+dependabot[bot]@users.noreply.github.com\",\"username\":\"dependabot[bot]\"},\"committer\":{\"name\":\"GitHub\",\"email\":\"noreply@github.com\",\"username\":\"web-flow\"},\"added\":[],\"removed\":[],\"modified\":[\".github/workflows/build.yml\"]}],\"head_commit\":{\"id\":\"c5ec2129a2a892156c8c97220e6059b9d47b7217\",\"tree_id\":\"2b50039faff054f37ce836e6d491db9912e38779\",\"distinct\":true,\"message\":\"Bump actions/setup-go (#13)\\n\\nBumps [actions/setup-go](https://github.com/actions/setup-go) from 8e57b58e57be52ac95949151e2777ffda8501267 to c0137caad775660c0844396c52da96e560aba63d.\\n- [Release notes](https://github.com/actions/setup-go/releases)\\n- [Commits](https://github.com/actions/setup-go/compare/8e57b58e57be52ac95949151e2777ffda8501267...c0137caad775660c0844396c52da96e560aba63d)\\n\\n---\\nupdated-dependencies:\\n- dependency-name: actions/setup-go\\n  dependency-version: c0137caad775660c0844396c52da96e560aba63d\\n  dependency-type: direct:production\\n...\\n\\nSigned-off-by: dependabot[bot] <support@github.com>\\nCo-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>\",\"timestamp\":\"2025-09-29T14:30:13+02:00\",\"url\":\"https://github.com/ctbur/ctbur.net/commit/c5ec2129a2a892156c8c97220e6059b9d47b7217\",\"author\":{\"name\":\"dependabot[bot]\",\"email\":\"49699333+dependabot[bot]@users.noreply.github.com\",\"username\":\"dependabot[bot]\"},\"committer\":{\"name\":\"GitHub\",\"email\":\"noreply@github.com\",\"username\":\"web-flow\"},\"added\":[],\"removed\":[],\"modified\":[\".github/workflows/build.yml\"]}}"

var fixHeaderEmptyCommits = map[string][]string{
	"Accept":                                 {"*/*"},
	"Content-Type":                           {"application/json"},
	"User-Agent":                             {"GitHub-Hookshot/2444035"},
	"X-Github-Delivery":                      {"0d27a2e6-9d30-11f0-8d1f-ed542b5a3474"},
	"X-Github-Event":                         {"push"},
	"X-Github-Hook-ID":                       {"572389829"},
	"X-Github-Hook-Installation-Target-ID":   {"758176159"},
	"X-Github-Hook-Installation-Target-Type": {"repository"},
	"X-Hub-Signature":                        {"sha1=9af99a1294c645ae70064a9bc87a88768adc5b62"},
	"X-Hub-Signature-256":                    {"sha256=80aed69e909cc9a5d6bd5db81eb062f9c2c6af6e0f3c40cf9aef7d5369b48209"},
}
var fixPayloadEmptyCommits = "{\"ref\":\"refs/heads/dependabot/github_actions/actions/setup-go-c0137caad775660c0844396c52da96e560aba63d\",\"before\":\"0431d43a2963efcc5db5c0c1f531e9fd570e7656\",\"after\":\"0000000000000000000000000000000000000000\",\"repository\":{\"id\":758176159,\"node_id\":\"R_kgDOLTDZnw\",\"name\":\"ctbur.net\",\"full_name\":\"ctbur/ctbur.net\",\"private\":false,\"owner\":{\"name\":\"ctbur\",\"email\":\"41328971+ctbur@users.noreply.github.com\",\"login\":\"ctbur\",\"id\":41328971,\"node_id\":\"MDQ6VXNlcjQxMzI4OTcx\",\"avatar_url\":\"https://avatars.githubusercontent.com/u/41328971?v=4\",\"gravatar_id\":\"\",\"url\":\"https://api.github.com/users/ctbur\",\"html_url\":\"https://github.com/ctbur\",\"followers_url\":\"https://api.github.com/users/ctbur/followers\",\"following_url\":\"https://api.github.com/users/ctbur/following{/other_user}\",\"gists_url\":\"https://api.github.com/users/ctbur/gists{/gist_id}\",\"starred_url\":\"https://api.github.com/users/ctbur/starred{/owner}{/repo}\",\"subscriptions_url\":\"https://api.github.com/users/ctbur/subscriptions\",\"organizations_url\":\"https://api.github.com/users/ctbur/orgs\",\"repos_url\":\"https://api.github.com/users/ctbur/repos\",\"events_url\":\"https://api.github.com/users/ctbur/events{/privacy}\",\"received_events_url\":\"https://api.github.com/users/ctbur/received_events\",\"type\":\"User\",\"user_view_type\":\"public\",\"site_admin\":false},\"html_url\":\"https://github.com/ctbur/ctbur.net\",\"description\":\"My personal website\",\"fork\":false,\"url\":\"https://api.github.com/repos/ctbur/ctbur.net\",\"forks_url\":\"https://api.github.com/repos/ctbur/ctbur.net/forks\",\"keys_url\":\"https://api.github.com/repos/ctbur/ctbur.net/keys{/key_id}\",\"collaborators_url\":\"https://api.github.com/repos/ctbur/ctbur.net/collaborators{/collaborator}\",\"teams_url\":\"https://api.github.com/repos/ctbur/ctbur.net/teams\",\"hooks_url\":\"https://api.github.com/repos/ctbur/ctbur.net/hooks\",\"issue_events_url\":\"https://api.github.com/repos/ctbur/ctbur.net/issues/events{/number}\",\"events_url\":\"https://api.github.com/repos/ctbur/ctbur.net/events\",\"assignees_url\":\"https://api.github.com/repos/ctbur/ctbur.net/assignees{/user}\",\"branches_url\":\"https://api.github.com/repos/ctbur/ctbur.net/branches{/branch}\",\"tags_url\":\"https://api.github.com/repos/ctbur/ctbur.net/tags\",\"blobs_url\":\"https://api.github.com/repos/ctbur/ctbur.net/git/blobs{/sha}\",\"git_tags_url\":\"https://api.github.com/repos/ctbur/ctbur.net/git/tags{/sha}\",\"git_refs_url\":\"https://api.github.com/repos/ctbur/ctbur.net/git/refs{/sha}\",\"trees_url\":\"https://api.github.com/repos/ctbur/ctbur.net/git/trees{/sha}\",\"statuses_url\":\"https://api.github.com/repos/ctbur/ctbur.net/statuses/{sha}\",\"languages_url\":\"https://api.github.com/repos/ctbur/ctbur.net/languages\",\"stargazers_url\":\"https://api.github.com/repos/ctbur/ctbur.net/stargazers\",\"contributors_url\":\"https://api.github.com/repos/ctbur/ctbur.net/contributors\",\"subscribers_url\":\"https://api.github.com/repos/ctbur/ctbur.net/subscribers\",\"subscription_url\":\"https://api.github.com/repos/ctbur/ctbur.net/subscription\",\"commits_url\":\"https://api.github.com/repos/ctbur/ctbur.net/commits{/sha}\",\"git_commits_url\":\"https://api.github.com/repos/ctbur/ctbur.net/git/commits{/sha}\",\"comments_url\":\"https://api.github.com/repos/ctbur/ctbur.net/comments{/number}\",\"issue_comment_url\":\"https://api.github.com/repos/ctbur/ctbur.net/issues/comments{/number}\",\"contents_url\":\"https://api.github.com/repos/ctbur/ctbur.net/contents/{+path}\",\"compare_url\":\"https://api.github.com/repos/ctbur/ctbur.net/compare/{base}...{head}\",\"merges_url\":\"https://api.github.com/repos/ctbur/ctbur.net/merges\",\"archive_url\":\"https://api.github.com/repos/ctbur/ctbur.net/{archive_format}{/ref}\",\"downloads_url\":\"https://api.github.com/repos/ctbur/ctbur.net/downloads\",\"issues_url\":\"https://api.github.com/repos/ctbur/ctbur.net/issues{/number}\",\"pulls_url\":\"https://api.github.com/repos/ctbur/ctbur.net/pulls{/number}\",\"milestones_url\":\"https://api.github.com/repos/ctbur/ctbur.net/milestones{/number}\",\"notifications_url\":\"https://api.github.com/repos/ctbur/ctbur.net/notifications{?since,all,participating}\",\"labels_url\":\"https://api.github.com/repos/ctbur/ctbur.net/labels{/name}\",\"releases_url\":\"https://api.github.com/repos/ctbur/ctbur.net/releases{/id}\",\"deployments_url\":\"https://api.github.com/repos/ctbur/ctbur.net/deployments\",\"created_at\":1708024625,\"updated_at\":\"2025-08-12T12:09:15Z\",\"pushed_at\":1759149014,\"git_url\":\"git://github.com/ctbur/ctbur.net.git\",\"ssh_url\":\"git@github.com:ctbur/ctbur.net.git\",\"clone_url\":\"https://github.com/ctbur/ctbur.net.git\",\"svn_url\":\"https://github.com/ctbur/ctbur.net\",\"homepage\":null,\"size\":105,\"stargazers_count\":0,\"watchers_count\":0,\"language\":\"Go\",\"has_issues\":true,\"has_projects\":true,\"has_downloads\":true,\"has_wiki\":false,\"has_pages\":false,\"has_discussions\":false,\"forks_count\":1,\"mirror_url\":null,\"archived\":false,\"disabled\":false,\"open_issues_count\":5,\"license\":null,\"allow_forking\":true,\"is_template\":false,\"web_commit_signoff_required\":false,\"topics\":[],\"visibility\":\"public\",\"forks\":1,\"open_issues\":5,\"watchers\":0,\"default_branch\":\"main\",\"stargazers\":0,\"master_branch\":\"main\"},\"pusher\":{\"name\":\"ctbur\",\"email\":\"41328971+ctbur@users.noreply.github.com\"},\"sender\":{\"login\":\"ctbur\",\"id\":41328971,\"node_id\":\"MDQ6VXNlcjQxMzI4OTcx\",\"avatar_url\":\"https://avatars.githubusercontent.com/u/41328971?v=4\",\"gravatar_id\":\"\",\"url\":\"https://api.github.com/users/ctbur\",\"html_url\":\"https://github.com/ctbur\",\"followers_url\":\"https://api.github.com/users/ctbur/followers\",\"following_url\":\"https://api.github.com/users/ctbur/following{/other_user}\",\"gists_url\":\"https://api.github.com/users/ctbur/gists{/gist_id}\",\"starred_url\":\"https://api.github.com/users/ctbur/starred{/owner}{/repo}\",\"subscriptions_url\":\"https://api.github.com/users/ctbur/subscriptions\",\"organizations_url\":\"https://api.github.com/users/ctbur/orgs\",\"repos_url\":\"https://api.github.com/users/ctbur/repos\",\"events_url\":\"https://api.github.com/users/ctbur/events{/privacy}\",\"received_events_url\":\"https://api.github.com/users/ctbur/received_events\",\"type\":\"User\",\"user_view_type\":\"public\",\"site_admin\":false},\"created\":false,\"deleted\":true,\"forced\":false,\"base_ref\":null,\"compare\":\"https://github.com/ctbur/ctbur.net/compare/0431d43a2963...000000000000\",\"commits\":[],\"head_commit\":null}"

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
	testCases := []struct {
		desc          string
		header        http.Header
		payload       string
		repoOwner     string
		repoName      string
		webhookSecret string
		wantHTTPCode  int
		wantBuild     *store.BuildMeta
	}{
		{
			desc:          "unrelated GitHub event",
			header:        headerSet(fixHeader, "X-GitHub-Event", "pull_request"),
			payload:       fixPayload,
			repoOwner:     "ctbur",
			repoName:      "ctbur.net",
			webhookSecret: fixWebhookSecret,
			wantHTTPCode:  http.StatusOK,
			wantBuild:     nil,
		},
		{
			desc:          "invalid commit SHA",
			header:        headerSet(fixHeader, "X-Hub-Signature-256", "sha256=bd0acfed478b06f17562fac13112e8c6337c413b948c49aac7de81d911733bfe"),
			payload:       strings.ReplaceAll(fixPayload, "c5ec2129a2a892156c8c97220e6059b9d47b7217", "not a commit SHA"),
			repoOwner:     "ctbur",
			repoName:      "ctbur.net",
			webhookSecret: fixWebhookSecret,
			wantHTTPCode:  http.StatusBadRequest,
			wantBuild:     nil,
		},
		{
			desc:          "missing signature",
			header:        headerDel(fixHeader, "X-Hub-Signature-256"),
			payload:       fixPayload,
			repoOwner:     "ctbur",
			repoName:      "ctbur.net",
			webhookSecret: fixWebhookSecret,
			wantHTTPCode:  http.StatusUnauthorized,
			wantBuild:     nil,
		},
		{
			desc:          "incorrect signature",
			header:        headerSet(fixHeader, "X-Hub-Signature-256", "sha256=120e0601c13771b59ff8e5f968619fea6eb0827d47e6082080b6bea6e37b6227"),
			payload:       fixPayload,
			repoOwner:     "ctbur",
			repoName:      "ctbur.net",
			webhookSecret: fixWebhookSecret,
			wantHTTPCode:  http.StatusUnauthorized,
			wantBuild:     nil,
		},
		{
			desc:          "repository not configured",
			header:        fixHeader,
			payload:       fixPayload,
			repoOwner:     "ctbur",
			repoName:      "other-repo",
			webhookSecret: fixWebhookSecret,
			wantHTTPCode:  http.StatusNotFound,
			wantBuild:     nil,
		},
		{
			desc:          "webhookSecret not configured",
			header:        fixHeader,
			payload:       fixPayload,
			repoOwner:     "ctbur",
			repoName:      "ctbur.net",
			webhookSecret: "",
			wantHTTPCode:  http.StatusInternalServerError,
			wantBuild:     nil,
		},
		{
			desc:          "correct configuration",
			header:        fixHeader,
			payload:       fixPayload,
			repoOwner:     "ctbur",
			repoName:      "ctbur.net",
			webhookSecret: fixWebhookSecret,
			wantHTTPCode:  http.StatusOK,
			wantBuild: &store.BuildMeta{
				Link:      "https://github.com/ctbur/ctbur.net/commit/c5ec2129a2a892156c8c97220e6059b9d47b7217",
				Ref:       "refs/heads/main",
				CommitSHA: "c5ec2129a2a892156c8c97220e6059b9d47b7217",
				Message: `Bump actions/setup-go (#13)

Bumps [actions/setup-go](https://github.com/actions/setup-go) from 8e57b58e57be52ac95949151e2777ffda8501267 to c0137caad775660c0844396c52da96e560aba63d.
- [Release notes](https://github.com/actions/setup-go/releases)
- [Commits](https://github.com/actions/setup-go/compare/8e57b58e57be52ac95949151e2777ffda8501267...c0137caad775660c0844396c52da96e560aba63d)

---
updated-dependencies:
- dependency-name: actions/setup-go
  dependency-version: c0137caad775660c0844396c52da96e560aba63d
  dependency-type: direct:production
...

Signed-off-by: dependabot[bot] <support@github.com>
Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>`,
				Author: "dependabot[bot]",
			},
		},
		{
			desc:          "branch deletion",
			header:        fixHeaderEmptyCommits,
			payload:       fixPayloadEmptyCommits,
			repoOwner:     "ctbur",
			repoName:      "ctbur.net",
			webhookSecret: fixWebhookSecret,
			wantHTTPCode:  http.StatusOK,
			wantBuild:     nil, // No build created on branch deletion
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Given
			cfg := config.Config{
				GitHub: config.GitHubConfig{
					WebhookSecret: tc.webhookSecret,
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
			webhook := http.Handler(handleGitHub(&c, &cfg))

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
