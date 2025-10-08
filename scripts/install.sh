#!/bin/bash
# Used to install the CI server on the remote host

set -e

REMOTE_HOST="${1:-root@ci.ctbur.net}"
BASE_DIR="${2:-/usr/local}"

# Set up an SSH agent to supply the key from an env var
if [ "$CI" = "true" ]; then
    echo "Running in CI mode: Starting SSH Agent and loading key from environment"

    if [ -z "$SSH_KNOWN_HOSTS" ]; then
        echo "ERROR: CI mode requires SSH_KNOWN_HOSTS environment variable." >&2
        exit 1
    fi

    # Write known_hosts to file
    mkdir -p ./.ssh && echo "$SSH_KNOWN_HOSTS" > "./.ssh/known_hosts"
    SSH_OPTS="-o StrictHostKeyChecking=yes -o UserKnownHostsFile=./.ssh/known_hosts"

    # Start the agent and set environment variables in the current script's shell
    eval "$(ssh-agent -s)" > /dev/null

    # Kill the agent when the script exits. $SSH_AGENT_PID is set by 'eval' above
    trap "kill $SSH_AGENT_PID; echo 'Stopped SSH Agent';" EXIT

    if [ -z "$SSH_HOST_KEY" ]; then
        echo "ERROR: CI mode requires SSH_HOST_KEY environment variable." >&2
        exit 1
    fi

    # Add key from environment variable, removing carriage returns
    echo "$SSH_HOST_KEY" | ssh-add -
else
    echo "Running in local mode: Using SSH key from ~/.ssh"
fi

echo "Copying files..."
rsync -e "ssh ${SSH_OPTS}" -avz "./build/ci-server" "${REMOTE_HOST}:${BASE_DIR}/bin/ci-server"
rsync -e "ssh ${SSH_OPTS}" -avz "./migrations" "./ui" "${REMOTE_HOST}:${BASE_DIR}/share/ci-server/"
echo ""

echo "Restarting service..."
ssh ${SSH_OPTS} "${REMOTE_HOST}" "systemctl restart ci.service"
echo ""

echo "Done"
