# Install prompt for Syspeek

Paste this file as-is to an AI agent with terminal access (Claude Code,
Cursor, etc.) to install **Syspeek** autonomously on Linux, macOS or Windows.

---

## Prompt

Install **Syspeek** from source on this machine. Syspeek is a real-time web-UI
system monitor written in Go (CPU, memory, disk, network, processes, sockets,
GPU, services, firewall). The official repository is
<https://github.com/neitanod/syspeek>.

Follow these steps in order and finish with a short report of what got
installed and how to start the app.

### 1. Detect the operating system

Identify whether you're on Linux, macOS or Windows before starting. The build
script and binary name differ per platform.

### 2. Verify prerequisites

- **Go 1.21 or newer** — check with `go version`.
- **git** — check with `git --version`.

If either is missing, ask the user to install it. Do not try to install Go
automatically.

### 3. Clone the repository

Pick a reasonable directory:

- Linux/macOS: `~/code/syspeek` or `~/src/syspeek`.
- Windows: `%USERPROFILE%\code\syspeek`.

```
git clone https://github.com/neitanod/syspeek.git <chosen-path>
```

If the directory already exists and is a valid clone, run `git pull --ff-only`
instead of re-cloning.

### 4. Build

**Linux/macOS:**

```bash
cd <chosen-path>
./build               # produces ./syspeek
```

**Windows (PowerShell):**

```powershell
cd <chosen-path>
.\build.ps1           # produces .\syspeek.exe
```

If PowerShell blocks the script:

```powershell
powershell -ExecutionPolicy Bypass -File .\build.ps1
```

### 5. (Optional) Install on PATH

- Linux/macOS: `sudo ln -sf "$(pwd)/syspeek" /usr/bin/syspeek` (asks for sudo;
  confirm with the user first).
- Windows: copy `syspeek.exe` to a directory that's already in `%PATH%`, e.g.
  `%USERPROFILE%\go\bin`.

### 6. (Optional) Configure authentication

Without a config file, run with `-p` for public read-only mode (no kill / no
renice). To enable auth, create:

- Linux/macOS: `~/.config/syspeek/config.json`
- Windows: `%USERPROFILE%\.config\syspeek\config.json`

```json
{
  "server":  { "host": "127.0.0.1", "port": 9876 },
  "auth":    { "username": "<user>", "password": "<password>" },
  "ui":      { "title": "<machine name>", "theme": "dark" }
}
```

### 7. Start it

The first invocation opens the user's browser automatically and exits when the
last tab closes. For a smoke test you want the persistent mode:

**Linux/macOS:**

```bash
./run --serve -p              # public read-only, no auto-shutdown
# or, if installed system-wide:
syspeek --serve -p
```

**Windows:**

```powershell
.\run.ps1 --serve -p
# or:
.\syspeek.exe --serve -p
```

### 8. Verify

From another terminal:

```
curl http://localhost:9876/api/cpu
curl http://localhost:9876/api/memory
curl http://localhost:9876/api/processes
```

All three must return HTTP 200 with JSON. The first call to each panel can
take a few seconds on Windows (PowerShell cold-start for the WMI-backed
collectors); follow-up reads are served from a short-TTL cache.

### 9. Report back

Tell the user:

- Path of the cloned repo and binary.
- Exact command to start the app again next time.
- URL to open in the browser (default <http://localhost:9876>).
- Whether you left the server running.
- Anything you skipped or that needs manual follow-up.

### Important notes

- **Do not touch `git config --global`.** If you need a git identity for any
  step, set it locally inside the repo and tell the user.
- **Do not install Go, git or any other system dependency without asking
  first.** Tell the user the exact command to run.
- On Windows, the `memory`, `services`, `users`, `firewall` and `gpu` panels
  shell out to PowerShell + WMI. They are intentionally cached server-side
  (3–30 s TTL) because each PowerShell invocation pays ~2 s of cold-start.
  This is expected behavior, not a bug.
- The default port is `9876`; if busy, syspeek tries the next 50 ports.
