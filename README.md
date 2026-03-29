# Timmy

Timmy is a Tailscale-first SSH connection manager built as a lightweight, scriptable alternative to Termius.

It is designed for teams that already trust Tailscale for network access and only need a shared source of truth for server metadata: Tailscale IPs, SSH usernames, and tags.

## Why Timmy

- CLI-first, so humans, scripts, and AI agents can all use the same interface
- Stores no SSH keys or passwords
- Uses Tailscale identity for backend access instead of app-specific auth
- Supports tags for filtering, grouping, and search
- Supports global search across server name, Tailscale IP, SSH username, and tags
- Keeps backend and CLI in separate Go modules so backend code is not shipped with the CLI binary

## Security Model

Timmy stores only:

- server name
- Tailscale IP
- SSH username
- tags
- audit metadata such as `created_by` and `updated_by`

Timmy does not store SSH private keys, passwords, or other login credentials.

The backend is intended to be reachable only over Tailscale. Requests are tied to Tailscale identity using `tsnet` and `LocalClient().WhoIs(...)`.

## Architecture

```mermaid
flowchart LR
  cli["TimmyCLI"]
  api["TimmyAPI_tsnet"]
  db["Postgres"]
  ssh["OpenSSH"]

  cli -->|"sync and search"| api
  api --> db
  cli -->|"ssh user@tailscaleIp"| ssh
```

## Repository Layout

- `cli/`: end-user CLI binary
- `backend/`: Tailscale-only API service
- `infra/`: local development infrastructure
- `dist/`: build outputs

## Requirements

- Go `1.26.1`
- Docker, for local Postgres development
- Tailscale installed and logged in
- OpenSSH available locally

## Installation

Tagged releases publish prebuilt CLI archives for macOS, Linux, and Windows via GitHub Releases.

For a public repository, users can download the matching archive from the latest release and place the `timmy` binary on their `PATH`.

For a private repository, the release workflow still works, but the assets are private too. Users need repository access and should download artifacts from the Releases page while signed in, or with the GitHub CLI:

```bash
gh release download v0.1.0 --repo OWNER/REPO --pattern 'timmy_*_darwin_arm64.tar.gz'
tar -xzf timmy_*_darwin_arm64.tar.gz
chmod +x timmy
mv timmy ~/.local/bin/timmy
```

`go install` is not currently the recommended install path for Timmy because the CLI module is not published under a fetchable public module path.

## Quickstart

### 1. Build the binaries

```bash
make build-cli build-backend
```

### 2. Start Postgres

```bash
make docker-up
```

This starts a local Postgres instance using `infra/docker-compose.yml`.

### 3. Configure the backend

Copy `.env.example` and export the variables you need:

```bash
export DATABASE_URL='postgres://timmy:timmy@localhost:5432/timmy?sslmode=disable'
export TIMMY_TAILNET_NAME='your-tailnet-name'
export TIMMY_HOSTNAME='timmyd'
export TIMMY_STATE_DIR="$PWD/backend/.data/tsnet"
export TS_AUTHKEY='tskey-auth-...'
```

Then run the backend:

```bash
./dist/timmyd
```

By default Timmy uses Tailscale HTTPS via `tsnet.ListenTLS`.

If your tailnet has not enabled Tailscale HTTPS yet, run Timmy in tailnet-only HTTP mode instead:

```bash
export TIMMY_USE_TLS='false'
export TIMMY_LISTEN_ADDR=':18080'
./dist/timmyd
```

### 4. Configure the CLI

Initialize the CLI with one or more Timmy servers:

```bash
./dist/timmy init https://timmyd.your-tailnet.ts.net --name prod
```

You can add multiple backends:

```bash
./dist/timmy init https://timmyd.team-a.ts.net --name team-a
./dist/timmy init https://timmyd.team-b.ts.net --name team-b
./dist/timmy servers
```

Switch the active backend for write operations:

```bash
./dist/timmy use team-a
```

`ls`, `search`, and `ssh` query all configured Timmy servers. `me`, `add`, `update`, and `rm` use the active server.

If you need to target a single backend without touching saved config, `TIMMY_API_URL` still works as a one-off override:

```bash
TIMMY_API_URL='https://timmyd.your-tailnet.ts.net' ./dist/timmy me
```

### 5. Verify access

```bash
./dist/timmy me
```

## CLI Usage

```bash
timmy init <server-url> [--name NAME]
timmy servers
timmy use <server-name>
timmy uninit <server-name>

timmy me [--json]
timmy add --name NAME --ip TAILSCALE_IP [--user root] [--tag TAG]... [--json]
timmy ls [--tag TAG]... [--json]
timmy search QUERY [--tag TAG]... [--limit N] [--json]
timmy ssh QUERY [--exact] [--first]
timmy update <id|query> [--name NAME] [--ip TAILSCALE_IP] [--user USER] [--tag TAG]... [--clear-tags] [--json]
timmy rm <id|query> [--json]
```

Notes:

- `ls`, `search`, and `ssh` fan out across all initialized Timmy servers
- `me`, `add`, `update`, and `rm` use the active server selected with `timmy use`

## Example Workflow

Add a server:

```bash
./dist/timmy add \
  --name prod-db-1 \
  --ip 100.64.0.10 \
  --user root \
  --tag prod \
  --tag database \
  --tag 203.0.113.10
```

Search by name, tag, or real IP stored as a tag:

```bash
./dist/timmy search prod
./dist/timmy search 203.0.113.10
```

Connect over SSH:

```bash
./dist/timmy ssh prod-db-1
```

## Configuration

### Backend environment variables

- `DATABASE_URL`: required Postgres connection string
- `TIMMY_TAILNET_NAME`: logical tailnet name stored in the database
- `TIMMY_HOSTNAME`: tsnet hostname, defaults to `timmyd`
- `TIMMY_STATE_DIR`: tsnet state directory, defaults to `.data/tsnet`
- `TIMMY_LISTEN_ADDR`: listen address, defaults to `:443`
- `TIMMY_USE_TLS`: defaults to `true`
- `TS_AUTHKEY`: optional auth key for non-interactive startup
- `TS_CLIENT_SECRET`: optional Tailscale OAuth client secret
- `TS_ADVERTISE_TAGS`: required when `TS_CLIENT_SECRET` is used

### CLI configuration

- `TIMMY_API_URL`: one-off backend URL override, useful for scripts or debugging
- `TIMMY_CONFIG`: custom path to the CLI config file that stores initialized servers and the active selection

## Development

Run tests:

```bash
make test
```

Build artifacts:

```bash
make build-cli build-backend
```

Publish a CLI release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Pushing a `v*` tag triggers GoReleaser and uploads release archives plus checksums to GitHub Releases.

Stop local Postgres:

```bash
make docker-down
```

## Status

Timmy is currently an MVP:

- single-tailnet oriented
- PostgreSQL-backed
- OpenSSH transport only
- optimized for internal team usage over Tailscale

## Open Source

Timmy is available under the MIT License. See `LICENSE`.

If you want to contribute, start with `CONTRIBUTING.md`.
