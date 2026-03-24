# repokv

A Git-based key-value storage service with HTTP API.

## Features

- **Git-backed storage**: Store key-value data in JSON files versioned by Git
- **Multi-tenant**: Support multiple repositories with separate API keys
- **HTTP API**: Simple POST-based API for updates
- **Hot reload**: Admin endpoint to reload configuration without restart
- **Concurrent safe**: Directory-level locking prevents conflicts
- **Automatic retry**: Failed operations retry with exponential backoff

## Quick Start

```bash
# Build
go build -o repokv .

# Run
./repokv
```

## Docker

### Pull Prebuilt Image

```bash
# GHCR
docker pull ghcr.io/yankeguo/repokv:latest

# Docker Hub
docker pull yankeguo/repokv:latest

# Quay
docker pull quay.io/yankeguo/repokv:latest
```

### Build Locally

```bash
docker build -t yankeguo/repokv:local .
```

### Run with Local Config/Data

The container image uses `WORKDIR /app`. If you keep default relative paths (`REPOKV_CONF_DIR=./conf`, `REPOKV_DATA_DIR=./data`), they are resolved from `/app` inside the container.

```bash
mkdir -p ./conf ./data

docker run --rm -p 8080:8080 \
  -e REPOKV_ADMIN_API_KEY=admin-api-key \
  -e REPOKV_CONF_DIR=/app/conf \
  -e REPOKV_DATA_DIR=/app/data \
  -v "$(pwd)/conf:/app/conf" \
  -v "$(pwd)/data:/app/data" \
  ghcr.io/yankeguo/repokv:latest
```

Then create repository config files in `./conf` (for example `./conf/myrepo.yaml`) and use the HTTP API as usual.

## Configuration

### Environment Variables

| Variable                  | Default         | Description                       |
| ------------------------- | --------------- | --------------------------------- |
| `PORT`                    | `8080`          | HTTP server port                  |
| `REPOKV_ADMIN_API_KEY`    | `admin-api-key` | Admin API key for reload          |
| `REPOKV_DATA_DIR`         | `./data`        | Directory for cloned repositories |
| `REPOKV_CONF_DIR`         | `./conf`        | Directory for repository configs  |
| `REPOKV_REPO_MAX_RETRIES` | `3`             | Max retries for git operations    |

### Repository Configuration

Create YAML files in `CONF_DIR` (one per repository):

```yaml
# conf/myrepo.yaml
api_key: secret-key-for-this-repo
url: https://github.com/user/repo.git
username: git-user
password: git-token
branch: main
path: data/config.json
git_user_name: repokv
git_user_email: repokv@example.com
```

Fields:

- `api_key` - API key for accessing this repository
- `url` - Git repository URL
- `username` / `password` - Git authentication credentials
- `branch` - Git branch to use
- `path` - Path to JSON file within the repository
- `git_user_name` / `git_user_email` - Git commit author

## API

### Update Key-Value

```bash
curl -X POST http://localhost:8080/myrepo \
  -H "X-API-Key: secret-key-for-this-repo" \
  -d "key1=value1" \
  -d "key2=value2"
```

### Reload Configuration (Admin)

```bash
curl -X POST http://localhost:8080/_reload \
  -H "X-API-Key: admin-api-key"
```

## How It Works

1. On startup, loads all repository configurations from `CONF_DIR`
2. On update request:
   - Acquire directory lock for the repository
   - Clone repository if not exists, or reset to remote state
   - Update JSON file with new key-value pairs
   - Commit and push changes
   - Release lock
3. On reload request: Re-scan `CONF_DIR` for configuration changes

## License

MIT License - see [LICENSE](LICENSE)
