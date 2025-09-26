#!/bin/bash

# Check if a URL was provided as an argument
if [ -z "$1" ]; then
  echo "Usage: $0 <github_commit_url>"
  echo "Ensure CI_USER and CI_PASSWORD environment variables are set."
  exit 1
fi

# Extract the URL from the first argument
url="$1"

# Extract owner, repo, and commit SHA using a single regular expression.
if [[ "$url" =~ ^https://github.com/([^/]+)/([^/]+)/commit/([^/]+) ]]; then
  owner="${BASH_REMATCH[1]}"
  repo="${BASH_REMATCH[2]}"
  sha="${BASH_REMATCH[3]}"
else
  echo "Error: Invalid GitHub commit URL format."
  exit 1
fi

echo "Fetching data for: $owner/$repo (SHA: $sha)"

# Get the repository ID
repo_info_json=$(gh api "repos/$owner/$repo")
repo_id=$(echo "$repo_info_json" | jq '.id')

# Get the commit details and construct the JSON payload
jq_filter='
{
    owner: "'$owner'",
    name: "'$repo'",
    link: .html_url,
    ref: "unknown",
    commit_sha: .sha,
    message: .commit.message,
    author: .author.login
}'
commit_payload=$(gh api "repos/$owner/$repo/commits/$sha" | jq --arg repo_id "$repo_id" "$jq_filter")

# Check if payload was successfully created
if [ -z "$commit_payload" ]; then
  echo "Error: Failed to create JSON payload."
  exit 1
fi

echo "-----------------------------------"
echo "JSON Payload to be posted:"
echo "$commit_payload"
echo "-----------------------------------"

# Post the JSON payload to the webhook using curl
curl -X POST \
     -H "Content-Type: application/json" \
     -H "Authorization: Basic $(echo -n $CI_USER:$CI_PASSWORD | base64)" \
     -d "$commit_payload" \
     "http://localhost:8000/webhook/manual"

echo ""
echo "Successfully posted to webhook."
