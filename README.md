**English** · [Español](README.es.md)

# Syspeek

Real-time system monitor with a web interface. Like `top` or `htop`, but in your browser with clickable cross-references between processes, users, network connections, and more.

![Syspeek Screenshot](screenshots/syspeek-full.png)

## Features

- **Processes**: View all running processes with CPU/memory usage, sorted and filterable. Click on a user to see all their processes, or on a PID to see its network connections.
- **Sockets**: See all TCP/UDP connections with local/remote addresses. Click on an IP to get geolocation and whois info, or on a PID to jump to the process.
- **CPU**: Real-time CPU usage per core with historical chart.
- **Memory**: RAM and swap usage with breakdown.
- **Disk**: Filesystem usage and I/O stats.
- **Network**: Interface traffic with real-time bandwidth graphs.
- **GPU**: NVIDIA GPU stats (if available).
- **Firewall**: View iptables/nftables rules (Linux) or Windows Firewall status.

### Cross-referencing

The key feature is **clickable links everywhere**:
- Click a PID in sockets view → jumps to that process
- Click a user → filters processes by that user
- Click an IP address → shows geolocation, hostname, whois
- Click a port → shows what service typically uses it

Everything is interconnected, making it easy to investigate "what's using my network?" or "what processes does this user have?".

## Installation

### Linux / macOS

```bash
git clone https://github.com/neitanod/syspeek.git
cd syspeek
./build
# Optional: install system-wide
sudo ln -sf $(pwd)/syspeek /usr/bin/syspeek
```

### Windows (PowerShell)

```powershell
git clone https://github.com/neitanod/syspeek.git
cd syspeek
.\build.ps1
.\run.ps1 -p
```

### Or install with an AI agent

If you use an agent with terminal access (Claude Code, Cursor, etc.), paste
this prompt and let it install everything for you:

<https://github.com/neitanod/syspeek/blob/main/install_prompt.md>

## Usage

```bash
# Opens browser automatically on port 9876
syspeek

# Server mode (no browser, useful for remote access)
syspeek --serve

# Public read-only mode (no auth required, useful for first run)
syspeek -p

# Custom port
syspeek --port 8080

# With config file
syspeek --config-file config.json
```

If port 9876 is busy, it automatically tries the next port (up to 50 attempts).

## Configuration

Copy `config.example.json` to `~/.config/syspeek/config.json`
(on Windows: `%USERPROFILE%\.config\syspeek\config.json`):

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 9876
  },
  "auth": {
    "username": "admin",
    "password": "your-password"
  },
  "ui": {
    "title": "My Server",
    "theme": "dark"
  }
}
```

Authentication is optional. Without it (or in `-p` mode), the interface is read-only (can't kill processes).

## Requirements

- Linux (reads from `/proc`), macOS, or Windows 10+
- Go 1.21+ (for building)
- NVIDIA drivers (optional, for GPU stats)

### Notes on the Windows backend

The Windows collectors use a mix of native Go (via `gopsutil`) and PowerShell
invocations of WMI (for memory, services, users, firewall and GPU). PowerShell
cold-start adds a few seconds to the very first read of each panel; subsequent
reads are served from a short-TTL in-process cache, so the live updates feel
the same as on Linux.

## License

MIT
