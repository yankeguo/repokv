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
