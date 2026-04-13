# Timmy Setup for JoyLink + DealSeek Developers

If you work on both JoyLink and DealSeek projects, this guide shows you how to set up Timmy to access servers from both teams simultaneously.

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
# Connect to BOTH servers
timmy init http://timmyd:8080 --name joylink
timmy init http://timmyd-dealseek:8080 --name dealseek

# See your configured servers
timmy servers

# Verify connection
timmy me
```

Your `timmy servers` output:
```
SERVER      URL                            ACTIVE
joylink     http://timmyd:8080             *
dealseek    http://timmyd-dealseek:8080
```

---

## How Multi-Server Works

### Read commands query ALL servers
These commands automatically search across both JoyLink and DealSeek:
- `timmy ls` - lists servers from both
- `timmy search` - searches both
- `timmy ssh` - finds servers in both

### Write commands use the ACTIVE server
These commands only affect whichever server is marked active:
- `timmy add` - adds to active server only
- `timmy update` - updates on active server only
- `timmy rm` - deletes from active server only

### Switch active server
```bash
# See which is active
timmy servers

# Switch to DealSeek
timmy use dealseek

# Switch to JoyLink
timmy use joylink
```

---

## Quickstart: Common Tasks

### List ALL servers (both teams)
```bash
timmy ls
```

Example output:
```
ID   NAME                  IP            USER    TAGS              SOURCE
1    joylink-prod          100.64.0.10   root    prod,api         joylink
2    joylink-staging       100.64.0.11   ubuntu  staging          joylink
3    redis-prod            100.64.0.20   root    prod,redis       joylink
4    notification-server   100.64.1.10   root    prod,notif       dealseek
5    analytics-backend     100.64.1.11   ubuntu  prod,analytics   dealseek
6    discord-bot           100.64.1.20   root    prod,bot         dealseek
```

Notice the `SOURCE` column shows which backend each server belongs to.

### Search across both teams
```bash
# Find all prod servers
timmy search prod

# Find all redis servers (might be in either team)
timmy search redis
```

### SSH to any server
```bash
# JoyLink server
timmy ssh joylink-prod

# DealSeek server
timmy ssh notification-server

# Works regardless of which is "active"
```

### Add a server to a specific team

```bash
# Add to JoyLink (make sure it's active first)
timmy use joylink
timmy add --name new-joylink-server --ip 100.64.0.99 --user root --tag prod

# Add to DealSeek
timmy use dealseek
timmy add --name new-dealseek-server --ip 100.64.1.99 --user root --tag prod
```

### One-off add without switching
```bash
# Add to DealSeek without changing active server
TIMMY_API_URL='http://timmyd-dealseek:8080' timmy add \
  --name temp-worker \
  --ip 100.64.1.88 \
  --user ubuntu
```

---

## Real-World Examples

### Morning routine: Check all prod servers
```bash
timmy ls --tag prod
```

### Deploy to JoyLink staging
```bash
timmy ssh joylink-staging
# On server:
cd /app && git pull && make deploy
```

### Check DealSeek notifications
```bash
timmy ssh notification-server
# On server:
journalctl -u notification-service -f
```

### Find a server you forgot the name of
```bash
# Search by partial name
timmy search redis

# Search by what it does
timmy search analytics

# Search by tag
timmy search --tag database
```

### Update server across teams
```bash
# Update JoyLink server
timmy use joylink
timmy update redis-prod --tag critical

# Update DealSeek server
timmy use dealseek
timmy update discord-bot --user ubuntu
```

---

## Command Reference

| Command | Scope | What it does |
|---------|-------|--------------|
| `timmy init <url> --name <name>` | Config | Add a timmyd server |
| `timmy servers` | Config | List configured servers |
| `timmy use <name>` | Config | Switch active server |
| `timmy uninit <name>` | Config | Remove a server |
| `timmy me` | Active | Show your identity |
| `timmy ls` | **All** | List servers from all backends |
| `timmy search <query>` | **All** | Search all backends |
| `timmy ssh <query>` | **All** | SSH (searches all backends) |
| `timmy add ...` | Active | Add server to active backend |
| `timmy update <query> ...` | Active | Update on active backend |
| `timmy rm <query>` | Active | Delete from active backend |

---

## Pro Tips

### Always know which server is active
```bash
timmy servers
```

### Use JSON for scripting
```bash
# All servers as JSON
timmy ls --json

# Pipe to jq
timmy ls --json | jq '.[] | select(.tags | contains(["prod"]))'
```

### Quick switch pattern
```bash
# Do something on DealSeek, switch back
timmy use dealseek && timmy add --name x --ip y --user z && timmy use joylink
```

### Environment override for CI/scripts
```bash
export TIMMY_API_URL='http://timmyd:8080'
timmy add --name ci-created-server --ip 100.64.0.77 --user deploy
```

---

## Troubleshooting

**"connection refused" to one server**
- Check Tailscale: `tailscale status`
- Ping the specific server: `ping timmyd` or `ping timmyd-dealseek`

**Added server to wrong team**
```bash
# Remove from wrong team
timmy use wrong-team
timmy rm server-name

# Add to correct team
timmy use correct-team
timmy add --name server-name --ip X --user Y
```

**Can't find a server?**
```bash
# List everything
timmy ls

# The SOURCE column tells you which backend it's in
```

---

## Server URLs

| Team | Server Name | URL |
|------|-------------|-----|
| JoyLink | `joylink` | `http://timmyd:8080` |
| DealSeek | `dealseek` | `http://timmyd-dealseek:8080` |

Both URLs are accessible only when connected to Tailscale.
