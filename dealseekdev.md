# Timmy Setup for DealSeek Developers

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
``

### Linux
```bash
gh release download --repo Carcraftz/timmy --pattern 'timmy_*_linux_amd64.tar.gz'
tar -xzf timmy_*_linux_amd64.tar.gz
sudo mv timmy /usr/local/bin/
```

## Quick Setup

```bash
# Connect to DealSeek's server
timmy init http://timmyd-dealseek:8080 --name dealseek

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
ID   NAME                  IP            USER    TAGS                    SOURCE
1    notification-server   100.64.1.10   root    prod,notifications     dealseek
2    analytics-backend     100.64.1.11   ubuntu  prod,analytics         dealseek
3    discord-bot           100.64.1.20   root    prod,bot               dealseek
4    expo-ota              100.64.1.30   root    prod,mobile            dealseek
```

### Search for servers
```bash
# Search by name
timmy search notification

# Search by tag
timmy search analytics

# Search by IP
timmy search 100.64.1.10
```

### SSH into a server
```bash
# By name
timmy ssh notification-server

# By partial name (picks first match)
timmy ssh notif --first

# Exact match only
timmy ssh notification-server --exact
```

### Add a new server
```bash
timmy add \
  --name dealseek-worker-1 \
  --ip 100.64.1.50 \
  --user ubuntu \
  --tag prod \
  --tag worker
```

### Update a server
```bash
# Change SSH user
timmy update dealseek-worker-1 --user root

# Add tags
timmy update dealseek-worker-1 --tag critical

# Change IP
timmy update dealseek-worker-1 --ip 100.64.1.51

# Clear all tags and set new ones
timmy update dealseek-worker-1 --clear-tags --tag prod --tag worker
```

### Delete a server
```bash
timmy rm dealseek-worker-1
```

---

## Real-World Examples

### Check notification server logs
```bash
timmy ssh notification-server
# Now on the server:
journalctl -u notification-service -f
```

### Deploy to analytics backend
```bash
timmy ssh analytics-backend
# Now on the server:
cd /app && git pull && docker-compose up -d
```

### Check Discord bot status
```bash
timmy ssh discord-bot
# Now on the server:
pm2 status
```

### List all production servers
```bash
timmy ls --tag prod
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
- Ping the server: `ping timmyd-dealseek`

**"timmy is not initiawill lized"**
- Run: `timmy init http://timmyd-dealseek:8080 --name dealseek`

**"multiple servers match"**
- Use `--first` to pick the first match, or be more specific with your query

**Can't find a server?**
- Use `timmy ls` to see all available servers
- Check spelling with `timmy search <partial-name>`

---

## DealSeek Server URL

```
http://timmyd-dealseek:8080
```

This is accessible only when connected to Tailscale.
