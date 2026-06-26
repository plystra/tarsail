# Tarsail

**Ship Docker Compose apps through unreliable networks.**

Tarsail is an open-source Docker Compose release bundler and SSH deployer by [Plystra](https://plystra.com).

It helps you build Docker images locally, package them into a portable release bundle, upload the bundle to a Linux server, load the images, and run your Compose app without requiring the target server to pull images from Docker Hub, GHCR, or a private registry during deployment.

> Phase 0 status: early development. Tarsail is not production-stable yet.

---

## Why Tarsail?

Deploying Docker Compose applications is usually simple:

```bash
docker compose up -d
```

Until the target server cannot reliably reach Docker Hub, GHCR, Quay, or your registry.

Tarsail is designed for restricted or unreliable network environments where the build machine can access dependencies, but the deployment server cannot.

Instead of asking the server to pull images, Tarsail does this:

```text
local build
  -> release bundle
  -> upload over SSH
  -> docker load on server
  -> docker compose up -d
```

No registry is required for deployment.

---

## What Tarsail Is

Tarsail is:

- a CLI tool
- a Docker Compose release bundler
- an SSH-based deployer
- a local-first deployment helper
- useful for restricted, unreliable, or offline-ish network environments

Tarsail is useful when:

- your server cannot pull from Docker Hub or GHCR reliably
- you do not want to maintain a private registry
- you deploy small or medium Docker Compose projects
- you want a repeatable `docker save -> upload -> docker load -> compose up` workflow
- you need simple rollback between bundled releases

---

## What Tarsail Is Not

Tarsail is not:

- a PaaS
- a Docker GUI
- a Kubernetes platform
- a Docker Swarm tool
- a CI/CD platform
- a monitoring system
- a secrets manager
- a registry
- a replacement for Portainer, Coolify, Dokploy, or Rancher

Tarsail intentionally focuses on one narrow job:

> Build and package a Docker Compose app somewhere that works, then reliably deliver it to a server where pulling images may not work.

---

## Phase 0 Scope

Phase 0 focuses on a boring but complete single-server deployment loop.

Supported:

- one local project
- one Docker Compose file
- one remote Linux server
- SSH deployment
- local image build
- release bundle creation
- remote `docker load`
- remote `docker compose up -d`
- status inspection
- log inspection
- rollback to previous release
- pruning old releases

Not supported in Phase 0:

- CI integration
- Web UI
- multi-server deployment
- remote builders
- registry push/pull workflows
- Kubernetes
- Docker Swarm
- automatic TLS
- Nginx/Caddy automation
- database backup
- Docker volume backup
- secret management

---

## How It Works

Tarsail creates a release bundle from your Compose project.

A bundle contains:

```text
manifest.json
compose.yaml
files/
  web-dist/
images/
  api.tar
  web.tar
```

Then Tarsail uploads the bundle to your server and applies it:

```text
/opt/my-app/
  current -> releases/20260618-143012-a7f3
  releases/
    20260618-143012-a7f3/
      manifest.json
      compose.yaml
      shared -> ../../shared
      files/
        web-dist/
      images/
        api.tar
        web.tar
  incoming/
  shared/
```

On the server, Tarsail runs:

```bash
docker load -i images/api.tar
docker load -i images/web.tar
docker compose --env-file current/.tarsail.env -f current/compose.yaml up -d
```

Your target server does not need to pull your application images from a registry during deployment.

---

## Installation

Tarsail is under early development.

Install or upgrade with the hosted installer.

Windows PowerShell:

```powershell
irm https://tarsail.plystra.com/install.ps1 | iex
```

Linux or macOS:

```bash
curl -fsSL https://tarsail.plystra.com/install.sh | sh
```

Verify:

```bash
tarsail version
```

Release assets are published automatically from GitHub Actions when changes are
pushed to `main`. Each release is tagged with a date plus commit hash, for
example `20260622-194500-a1b2c3d4e5f6`, and marked as the GitHub latest release.

For development builds from source:

```bash
git clone https://github.com/plystra/tarsail.git
cd tarsail
go build -o tarsail ./cmd/tarsail
```

Move the binary somewhere in your `PATH`:

```bash
sudo mv ./tarsail /usr/local/bin/tarsail
```

---

## Quick Start

### 1. Prepare a Docker Compose project

Your Compose services should use explicit image tags.

Good:

```yaml
services:
  api:
    build: ./api
    image: my-app-api:local

  web:
    build: ./web
    image: my-app-web:${TARSAIL_RELEASE_ID:-local}
```

Not supported in Phase 0:

```yaml
services:
  api:
    build: ./api
```

Tarsail needs stable image tags so it can save, load, and run the correct images.
During `deploy`, Tarsail provides `TARSAIL_RELEASE_ID` to local and remote
Docker Compose commands, so image tags can follow the release ID without manual
editing.

---

### 2. Initialize Tarsail

```bash
tarsail init
```

This creates:

```text
tarsail.yml
```

Example:

```yaml
project: my-app

target:
  name: prod
  host: example.com
  user: deploy
  port: 22
  path: /opt/my-app

compose:
  file: compose.yaml
  env_file:
    source: .deploy/prod.env
    target: shared/.env

build:
  steps:
    - name: Build web assets
      run: npm run build:web

deploy:
  keep_releases: 3

files:
  - source: apps/web/dist
    target: files/web-dist

secrets:
  - source: .deploy/htpasswd
    target: shared/auth/htpasswd
    mode: 600
```

For documentation examples, use placeholder hosts such as `example.com` or documentation-only IP ranges such as `192.0.2.10`. Do not commit real server addresses, real domains, credentials, or private deployment paths to public examples.

---

### 3. Check your environment

```bash
tarsail doctor
```

Use a specific SSH private key when your server is not using the default SSH
identity:

```bash
tarsail --identity-file ~/.ssh/my-deploy-key doctor
```

If the server only allows password login, Tarsail can ask once and reuse that
password for all remote commands and file uploads in the current run:

```bash
tarsail --ask-password deploy
```

Password mode uses your local `~/.ssh/known_hosts` for host-key verification.
Connect once with your normal `ssh user@example.com` flow first if the host key
is not already trusted.

Tarsail checks:

- local Docker availability
- local Docker Compose availability
- Compose file validity
- explicit image tags
- SSH connectivity
- remote Docker availability
- remote Docker Compose availability
- remote target path permissions

---

### 4. Deploy

```bash
tarsail deploy
```

Tarsail will:

1. check the local Docker and Compose environment
2. run configured build steps, if any
3. check the remote server and upload configured secrets
4. build your Compose images locally
5. save images into tar files and create a release bundle
6. upload and extract the bundle into a new release directory
7. run `docker load`
8. activate the new release
9. run `docker compose up -d`
10. show remote Compose status

---

### 5. Check status

```bash
tarsail status
```

Equivalent remote operation:

```bash
cd /opt/my-app/current
docker compose ps
```

---

### 6. Read logs

```bash
tarsail logs
```

For a specific service:

```bash
tarsail logs api
```

---

### 7. Roll back

```bash
tarsail rollback
```

Rollback restores the previous release's:

- Compose file
- bundled images
- active release pointer

Rollback does not restore:

- databases
- Docker volumes
- bind-mounted files
- external services
- remote environment files

---

### 8. Prune old releases

```bash
tarsail prune
```

Tarsail only prunes old non-current releases under:

```text
<target.path>/releases
```

It does not delete Docker volumes, databases, bind mounts, or files outside the Tarsail-managed release directory.

---

## Commands

| Command | Description |
|---|---|
| `tarsail init` | Create a minimal `tarsail.yml` |
| `tarsail doctor` | Check local and remote deployment readiness |
| `tarsail deploy` | Build, bundle, upload, load, and start the app |
| `tarsail status` | Show remote Compose status |
| `tarsail logs` | Show remote Compose logs |
| `tarsail rollback` | Roll back to the previous release |
| `tarsail prune` | Delete old non-current releases |
| `tarsail version` | Show installed Tarsail version |

Global options:

| Option | Description |
|---|---|
| `--config <path>` | Path to `tarsail.yml` |
| `--identity-file <path>` | SSH private key file to pass to `ssh` and `scp` |
| `--ssh-key <path>` | Alias for `--identity-file` |
| `--ask-password` | Prompt once for the remote user's SSH password |
| `--verbose` | Show verbose command output where available |
| `--yes` | Answer yes to confirmation prompts |
| `-v`, `--version` | Show Tarsail version and exit |

---

## Configuration

Default config file:

```text
tarsail.yml
```

Minimal example:

```yaml
project: my-app

target:
  name: prod
  host: example.com
  user: deploy
  port: 22
  path: /opt/my-app

compose:
  file: compose.yaml

deploy:
  keep_releases: 3
```

### `project`

Project name used in bundle names and remote release metadata.

Allowed characters:

```text
a-z
0-9
-
_
```

### `target`

The remote Linux server to deploy to.

Phase 0 supports exactly one target.

### `compose.file`

Path to the Docker Compose file relative to the project root.

### `compose.env_file`

Optional env file used by remote Docker Compose.

If `source` is set, Tarsail uploads that local file over SSH/SCP during deploy.
If `source` is omitted, the file must already exist on the server under
`shared/`.

Tarsail never prints env file contents.

### `build.steps`

Optional local commands to run before Tarsail creates the release bundle.

Use this for generated release files such as Vite, Astro, Next static export,
or other static asset output that is later copied through `files`.

Example:

```yaml
build:
  steps:
    - name: Build web dist
      run: npm run build:web

files:
  - source: apps/web/dist
    target: files/web-dist
```

Build steps run only during `deploy`, before Tarsail contacts the remote server,
builds Compose images, or creates the bundle. They receive
`TARSAIL_RELEASE_ID=<release-id>`.

### `files`

Optional explicit non-secret files or directories to copy into each release
under `files/`.

Use this for static assets, Nginx config, and other release-owned files that
should roll back with the Compose file and images.

If a file source is generated, configure `build.steps` to create it. Tarsail
checks `files` sources after build steps and before uploading a bundle.

### `secrets`

Optional explicit secret files to upload over SSH/SCP into the remote `shared/`
directory.

Secrets are not stored in the release bundle and do not roll back with releases.
Tarsail does not generate, rotate, back up, or manage secrets beyond copying
the configured files and applying the configured file mode.

### `deploy.keep_releases`

Number of releases to keep when pruning.

Default:

```yaml
keep_releases: 3
```

---

## Requirements

### Local Machine

Required:

- Docker
- Docker Compose v2
- SSH access to the target server

Supported local systems:

- macOS
- Linux
- Windows

### Remote Server

Required:

- Linux
- SSH
- Docker
- Docker Compose v2
- permission to run Docker commands
- writable target path

Tarsail does not install Docker for you in Phase 0.

---

## Security Notes

Tarsail uses SSH to connect to your server.
By default, Tarsail uses your normal `ssh` and `scp` configuration. You can pass
`--identity-file` for a specific private key, or `--ask-password` to enter the
remote user's password once for the current command.

Phase 0 does not:

- store SSH passwords
- store private key contents
- upload private keys
- manage server users
- generate, rotate, encrypt, back up, or validate secrets
- automatically bundle `.env` files
- automatically discover or collect secrets

You are responsible for creating and maintaining production secrets and
environment files. Tarsail only transfers explicit files that you configure.

Tarsail intentionally avoids becoming a secret management system.

### Public Documentation Hygiene

Public examples must not include:

- real server IP addresses
- real private domains
- real usernames tied to private infrastructure
- credentials
- private key paths
- production database URLs
- tokens
- `.env` contents

Use placeholders such as:

```text
example.com
deploy@example.com
/opt/my-app
192.0.2.10
```

---

## Roadmap

### Phase 0

- [ ] `tarsail init`
- [ ] `tarsail doctor`
- [ ] `tarsail deploy`
- [ ] release bundle format
- [ ] SSH upload
- [ ] remote `docker load`
- [ ] remote `docker compose up -d`
- [ ] `tarsail status`
- [ ] `tarsail logs`
- [ ] `tarsail rollback`
- [ ] `tarsail prune`

### Later Phases

Possible future features:

- CI-friendly non-interactive mode
- GitHub Actions examples
- GitLab CI examples
- zstd compression
- upload progress
- resumable upload
- deployment hooks
- remote builder support
- multiple targets
- webhook notifications
- optional env file handling
- checksums and bundle verification
- signed release bundles

These are not part of Phase 0.

---

## Design Principles

### 1. Stay small

Tarsail should solve one deployment problem well.

### 2. Do not require a registry

A registry can be useful, but it should not be required for the core deployment loop.

### 3. Prefer boring infrastructure

SSH, tar, Docker, and Compose are enough for Phase 0.

### 4. Be explicit

Tarsail should fail clearly instead of guessing dangerously.

### 5. Avoid platform creep

Tarsail should not become a PaaS, CI platform, monitoring platform, or Kubernetes abstraction.

---

## Contributing

Contributions are welcome, especially around:

- Docker Compose compatibility
- SSH reliability
- bundle safety
- cross-platform behavior
- documentation
- error messages
- tests

Before adding a feature, please check whether it fits the current phase.

For Phase 0, the key question is:

> Does this directly improve the single-server offline Docker Compose deployment loop?

If not, it probably belongs in a later phase.

---

## Development

Build locally:

```bash
go build -o tarsail ./cmd/tarsail
```

Run tests:

```bash
go test ./...
```

Recommended repository layout:

```text
.
├── cmd/
│   └── tarsail/
├── internal/
│   ├── bundle/
│   ├── compose/
│   ├── config/
│   ├── docker/
│   ├── remote/
│   ├── release/
│   └── ui/
├── README.md
├── go.mod
└── go.sum
```

---

## License

Tarsail is planned to be released as open source.

Recommended license:

```text
Apache-2.0
```

The final license should be confirmed before the first public release.

---

## Brand

Tarsail is an open-source project by [Plystra](https://plystra.com).

Website:

```text
https://tarsail.plystra.com
```

---

## Name

**Tarsail** combines:

```text
tar   -> archive, bundle, package
sail  -> ship, transport, deliver
```

Tarsail packages your Compose app and sails it across unreliable networks.
