# Timmy Setup for JoyLink Developers

Timmy is our CLI tool for managing SSH connections to servers. It uses Tailscale for secure access - no passwords or SSH keys to manage.

## Prerequisites

1. **Tailscale installed and connected**
   ```bash
   tailscale status
   ```
   If not connected, run `tailscale up` and log in.

2. **Access to the `aveekm.github` tailnet**
   - Request an invite from your team lead if you haven't joined yet

## Installation

### macOS (Apple Silicon)
```bash
gh release download --repo Carcraftz/timmy --pattern 'timmy_*_darwin_arm64.tar.gz'
tar -xzf timmy_*_darwin_arm64.tar.gz
sudo mv timmy /usr/local/bin/
```

### macOS (Intel)
```bash
gh release download --repo Carcraftz/timmy --pattern 'timmy_*_darwin_amd64.tar.gz'
tar -xzf timmy_*_darwin_amd64.tar.gz
sudo mv timmy /usr/local/bin/
```

### Linux
```bash
gh release download --repo Carcraftz/timmy --pattern 'timmy_*_linux_amd64.tar.gz'
tar -xzf timmy_*_linux_amd64.tar.gz
sudo mv timmy /usr/local/bin/
```

## Quick Setup

```bash
# Connect to JoyLink's server
timmy init http://timmyd:8080 --name joylink

# Verify it works
timmy me
```

You should see:
```
Login:    your-email@example.com
Display:  Your Name
Node:     your-laptop
Tailnet:  aveekm.github
```

---

## Quickstart: Common Tasks

### List all servers
```bash
timmy ls
```

Example output:
```
ID   NAME              IP            USER    TAGS                SOURCE
1    joylink-prod      100.64.0.10   root    prod,api           joylink
2    joylink-staging   100.64.0.11   ubuntu  staging,api        joylink
3    redis-prod        100.64.0.20   root    prod,redis         joylink
4    llm-router        100.64.0.30   root    prod,llm           joylink
```

### Search for servers
```bash
# Search by name
timmy search prod

# Search by tag
timmy search redis

# Search by IP
timmy search 100.64.0.10
```

### SSH into a server
```bash
# By name
timmy ssh joylink-prod

# By partial name (picks first match)
timmy ssh prod --first

# Exact match only
timmy ssh joylink-prod --exact
```

### Add a new server
```bash
timmy add \
  --name joylink-worker-1 \
  --ip 100.64.0.50 \
  --user ubuntu \
  --tag prod \
  --tag worker
```

### Update a server
```bash
# Change SSH user
timmy update joylink-worker-1 --user root

# Add tags
timmy update joylink-worker-1 --tag critical

# Change IP
timmy update joylink-worker-1 --ip 100.64.0.51

# Clear all tags and set new ones
timmy update joylink-worker-1 --clear-tags --tag prod --tag worker
```

### Delete a server
```bash
timmy rm joylink-worker-1
```

---

## Real-World Examples

### Deploy workflow: SSH to staging, run deploy
```bash
timmy ssh joylink-staging
# Now on the server:
cd /app && git pull && make deploy
```

### Find and connect to Redis
```bash
timmy search redis
# Shows: redis-prod (100.64.0.20)

timmy ssh redis-prod
# Now on Redis server:
redis-cli ping
```

### Check all production servers
```bash
timmy ls --tag prod
```

### Script: Run command on a server
```bash
# Get server IP and SSH in one line
timmy ssh llm-router -c "systemctl status llm-router"
```

### JSON output for scripting
```bash
# Get all servers as JSON
timmy ls --json

# Get your identity as JSON
timmy me --json
```

---

## Command Reference

| Command | What it does |
|---------|--------------|
| `timmy init <url> --name <name>` | Connect to a timmyd server |
| `timmy me` | Show your Tailscale identity |
| `timmy ls` | List all servers |
| `timmy ls --tag prod` | List servers with tag "prod" |
| `timmy search <query>` | Search servers by name/IP/tag |
| `timmy ssh <query>` | SSH to a server |
| `timmy ssh <query> --first` | SSH to first matching server |
| `timmy add --name X --ip Y --user Z` | Add a new server |
| `timmy update <query> --name X` | Update server details |
| `timmy rm <query>` | Delete a server |

---

## Troubleshooting

**"connection refused"**
- Check Tailscale is running: `tailscale status`
- Ping the server: `ping timmyd`

**"timmy is not initialized"**
- Run: `timmy init http://timmyd:8080 --name joylink`

**"multiple servers match"**
- Use `--first` to pick the first match, or be more specific with your query

**Can't find a server?**
- Use `timmy ls` to see all available servers
- Check spelling with `timmy search <partial-name>`

---

## JoyLink Server URL

```
http://timmyd:8080
```

This is accessible only when connected to Tailscale.
