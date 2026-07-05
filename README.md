# CI Server

A work-in-progress CI server that aims to keep things simple and keep files cached locally.

## Goals

Most CI servers run stateless build jobs, so each build must either start from scratch or download large amounts of cached data (packages, container images, build artifacts, and so on). Even with a well-tuned cache, this makes builds slower than on your local machine because of the network transfers involved.

This server takes a different approach: it is stateful and keeps build files cached on local disk. Every branch starts from a copy of the build files produced by the last completed build of the default branch, which is the main mechanism for speed.

Its goals, in order of priority:

- **Builds as fast as local**
- **Radically simple**
  - No programming pipelines in YAML: You get one command, use an appropriate build tool.
  - Single-server design: Runs on a single server with local disk storage only.
  - Single binary: The binary contains all you need, no containers needed.

## Architecture

The CI server consists of two main components: the scheduler (running inside the server process) and the builders (separate processes).

```mermaid
flowchart TD
    GitHub -->|push webhook| Scheduler
    Scheduler -->|reports commit status| GitHub

    subgraph Server[Server Process]
        Scheduler
    end

    Scheduler <-->|queues & polls builds| DB[(Database)]
    Scheduler -->|spawns| Builder[Builder Process]

    Builder -->|writes logs & exit code| Disk[(Local Disk)]
    Disk -->|cached build files| Builder
    Scheduler -->|reads results & cleans up| Disk
```

### Scheduler (Server Process)

The scheduler runs inside the main server process. It:

- Receives webhooks from GitHub for push events
- Polls for pending builds and spawns builder processes
- Monitors running builders and collects results
- Reports build status back to GitHub commits
- Cleans up unused build directories

### Builder (Separate Processes)

Builder processes run independently from the server. This design allows:

- Server upgrades without stopping running builds
- Crash isolation between builds
- Independent resource management

Each builder:

1. Copies cached files from the last successful default branch build
2. Checks out the repository at the specified commit
3. Runs the build command in a sandboxed environment
4. Runs the deploy command (only on default branch, only if build succeeds)
5. Writes the exit code for the scheduler to collect

## How Builds Work

### Build Lifecycle

1. **Webhook received** - GitHub sends a push event
2. **Build queued** - A pending build record is created in the database
3. **Builder spawned** - The scheduler starts a new builder process
4. **Cache copied** - Build files from the last default branch build are copied (if available)
5. **Checkout** - Repository is fetched and checked out at the commit SHA
6. **Build command** - The configured build command runs in a sandbox
7. **Deploy command** - If on default branch and build succeeded, deploy runs
8. **Results collected** - Scheduler reads exit code and updates build status
9. **Status reported** - Commit status is updated on GitHub

### Caching Strategy

- Only builds on the default branch create cache
- All branches start from a copy of the default branch cache
- Cache is stored locally on disk for maximum speed
- Old build directories are automatically cleaned up

### Build Environment

During builds, the following environment variables are available:

- `CI=true` - Indicates running in CI environment
- `PATH` - Inherited from server configuration
- `HOME` - Set to the build directory
- Any custom `env_vars` from repo configuration
- Any decrypted `build_secrets` or `deploy_secrets`

## Sandboxing

Build commands run inside a [bubblewrap](https://github.com/containers/bubblewrap) sandbox with the following restrictions:

- `--unshare-all --share-net` - Isolated namespaces except network
- `--ro-bind / /` - Root filesystem is read-only
- `--dev /dev` - Fresh /dev mount
- `--tmpfs /tmp` - Fresh /tmp mount
- `--bind <build-dir> <build-dir>` - Build directory is writable

This ensures builds cannot modify the host system while still allowing network access for downloading dependencies.

### Required System Configuration

Add to `/etc/sysctl.conf`:

```
kernel.unprivileged_userns_clone = 1
kernel.apparmor_restrict_unprivileged_userns = 0
```

## GitHub App Configuration

The CI server operates as a GitHub App. You need to create a GitHub App with:

### Required Permissions

- **Repository permissions:**
  - Contents: Read (to clone repositories)
  - Commit statuses: Read & Write (to report build status)
- **Account permissions:**
  - Email addresses: Read (for user authentication)

### Webhook Events

- Push events

### OAuth Configuration

- Enable "Request user authorization (OAuth) during installation"
- Set callback URL to `<host_url>/auth/callback`

### App Settings

- Generate a private key and save it (referenced by `private_key_path`)
- Note the App ID (for `app_id` config)
- Note the Client ID (for `client_id` config)
- Note the Client Secret (encrypt for `encrypted_client_secret`)
- Set a webhook secret (encrypt for `encrypted_webhook_secret`)

## Configuration

### Configuration File (TOML)

```toml
data_dir = "/var/lib/ci-server/data"
host_url = "https://ci.example.com"

[github]
app_id = 123456
client_id = "Iv1.abc123"
encrypted_client_secret = "<encrypted>"
encrypted_webhook_secret = "<encrypted>"
private_key_path = "/path/to/private-key.pem"
authorized_installations = [12345678]
authorized_users = ["username1", "username2"]

[[repos]]
owner = "myorg"
name = "myrepo"
default_branch = "main"
build_command = ["make", "build", "test"]
deploy_command = ["make", "deploy"]
env_vars = { NODE_ENV = "production" }
encrypted_build_secrets = { NPM_TOKEN = "<encrypted>" }
encrypted_deploy_secrets = { DEPLOY_KEY = "<encrypted>" }
```

### Encrypting Secrets

Use the provided script to encrypt secrets:

```bash
export CI_SERVER_SECRET_KEY="$(openssl rand -hex 32)"
./scripts/encrypt-secret.sh "my-secret-value"
```

The `CI_SERVER_SECRET_KEY` must be a 64-character hexadecimal string (32 bytes for AES-256).

### Required Environment Variables

| Variable                 | Description                                              |
| ------------------------ | -------------------------------------------------------- |
| `CI_SERVER_SECRET_KEY`   | 64-character hex string for encrypting secrets in config |
| `CI_SERVER_AUTH_KEY`     | 64-character hex string for session encryption           |
| `CI_SERVER_POSTGRES_URL` | PostgreSQL connection string (production)                |
| `CI_SERVER_DEV`          | Set to `1` for development mode                          |

## Running the Server

```bash
./ci-server --config /path/to/config-dir/
```

The config directory should contain:

- `ci-config.toml` - Main configuration file
- The GitHub App private key file (path specified in config)

### Database Setup

The server requires PostgreSQL. Connection is configured via `CI_SERVER_POSTGRES_URL`:

```bash
export CI_SERVER_POSTGRES_URL="postgres://user:pass@localhost:5432/ci?sslmode=disable"
```

Migrations are applied automatically on startup.

### Development Mode

Set `CI_SERVER_DEV=1` to enable:

- Embedded PostgreSQL (no external database needed)
- Debug logging
- Manual webhook endpoint at `/webhook/manual`

## Deployment

### Dependencies

Required binaries:

- `bwrap` (bubblewrap) - For build sandboxing
- `cp` - For cache copying
- `git` - For repository checkout

### Systemd Service

A systemd service file is provided at `ci.service`:

```bash
sudo cp ci.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable ci.service
sudo systemctl start ci.service
```

The service is configured with:

- `KillMode=process` - Only kills the server, allowing builders to finish
- `ProtectHome=true` and `ProtectSystem=full` - Additional security hardening
- `NoNewPrivileges=true` - Prevents privilege escalation

### Installation Script

The `scripts/install.sh` script deploys to a remote server:

```bash
# Local deployment
./scripts/install.sh

# CI deployment (requires SSH_KNOWN_HOSTS and SSH_HOST_KEY env vars)
CI=true ./scripts/install.sh
```

---

Using [Fontawesome](https://fontawesome.com/) icons.
