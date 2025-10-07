#!/bin/bash
# Used to install the CI server on the remote host

set -e

REMOTE_HOST="${1:-root@ci.ctbur.net}"
BASE_DIR="${2:-/usr/local}"

# Set up an SSH agent to supply the key from an env var
if [ "$CI" = "true" ]; then
    echo "Running in CI mode: Starting SSH Agent and loading key from environment"

    # Start the agent and set environment variables in the current script's shell
    eval "$(ssh-agent -s)" > /dev/null

    # Kill the agent when the script exits. $SSH_AGENT_PID is set by 'eval' above
    trap "echo 'Stopping SSH Agent'; kill $SSH_AGENT_PID;" EXIT

    if [ -z "$HOST_SSH_KEY" ]; then
        echo "ERROR: CI mode requires HOST_SSH_KEY environment variable." >&2
        exit 1
    fi

    # Add key from environment variable, removing carriage returns
    echo "$SSH_KEY" | tr -d '\r' | ssh-add -
else
    echo "Running in local mode: Using SSH key from ~/.ssh"
fi

echo "Copying files..."
rsync -avz "./build/ci-server" "${REMOTE_HOST}:${BASE_DIR}/bin/ci-server"
rsync -avz "./migrations" "./ui" "${REMOTE_HOST}:${BASE_DIR}/share/ci-server/"
echo ""

echo "Restarting service..."
ssh -t "${REMOTE_HOST}" "systemctl restart ci.service"
echo ""

echo "Done"
