#!/bin/bash

# Check if a URL was provided
if [ -z "$1" ]; then
  echo "Usage: $0 <github_commit_url>"
  exit 1
fi

url="$1"

# Regex to extract owner, repo, and sha
if [[ "$url" =~ ^https://github.com/([^/]+)/([^/]+)/commit/([^/]+) ]]; then
  owner="${BASH_REMATCH[1]}"
  repo="${BASH_REMATCH[2]}"
  sha="${BASH_REMATCH[3]}"
else
  echo "Error: Invalid GitHub commit URL format."
  exit 1
fi

echo "Fetching data for: $owner/$repo (SHA: $sha)"

# 1. Get basic Repo Info (for ID and Default Branch name)
repo_json=$(gh api "repos/$owner/$repo")
repo_id=$(echo "$repo_json" | jq -r '.id')
default_branch=$(echo "$repo_json" | jq -r '.default_branch')

# 2. Find the Branch
# Strategy A: Is this commit the TIP (Head) of any branch?
# This API endpoint explicitly lists branches where this commit is the latest one.
head_branch=$(gh api "repos/$owner/$repo/commits/$sha/branches-where-head" --jq '.[0].name')

if [ -n "$head_branch" ] && [ "$head_branch" != "null" ]; then
  target_ref="refs/heads/$head_branch"
  echo "Match found: Commit is the HEAD of '$head_branch'."

else
  # Strategy B: Is the commit INSIDE the default branch?
  # We compare the commit (base) to the default branch (head).
  # If the default branch is 'ahead' or 'identical', it contains our commit.
  compare_status=$(gh api "repos/$owner/$repo/compare/$sha...$default_branch" --jq '.status')

  if [[ "$compare_status" == "ahead" || "$compare_status" == "identical" ]]; then
    target_ref="refs/heads/$default_branch"
    echo "Match found: Commit is merged into '$default_branch'."
  else
    # Strategy C: Fallback
    # The commit is on a non-default branch and is not the tip.
    # Without cloning the repo, finding the exact branch name is expensive (requires checking all branches).
    # We fallback to the default branch to ensure the webhook doesn't fail.
    target_ref="refs/heads/$default_branch"
    echo "No direct branch tip match. Defaulting ref to: $default_branch"
  fi
fi

# 3. Create JSON Payload
jq_filter='
{
    owner: "'$owner'",
    name: "'$repo'",
    link: .html_url,
    ref: $ref_arg,
    commit_sha: .sha,
    message: .commit.message,
    author: .author.login
}'

# We use --arg to safely pass the bash variable $target_ref into jq
commit_payload=$(gh api "repos/$owner/$repo/commits/$sha" | jq --arg ref_arg "$target_ref" "$jq_filter")

if [ -z "$commit_payload" ]; then
  echo "Error: Failed to create JSON payload."
  exit 1
fi

echo "-----------------------------------"
echo "JSON Payload to be posted:"
echo "$commit_payload"
echo "-----------------------------------"

# 4. Post to Webhook
curl -X POST \
     -H "Content-Type: application/json" \
     -d "$commit_payload" \
     "http://localhost:8000/webhook/manual"

echo ""
echo "Successfully posted to webhook."
