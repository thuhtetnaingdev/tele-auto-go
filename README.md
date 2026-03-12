# tele-auto-go monorepo

This project now has two apps:

- `backend/` - Go Telegram auto-reply worker + control API.
- `frontend/` - Vite + React + TypeScript dashboard (shadcn-style UI).

Run both together from repo root:

```bash
make dev
```

## Production One-Liner Install

1. Build release artifacts locally:

```bash
make release-build VERSION=v1.0.0
```

This creates:
- `release/tele-auto-go_v1.0.0_linux_amd64.tar.gz`
- `release/tele-auto-go_v1.0.0_linux_arm64.tar.gz`
- `release/tele-auto-go-web_v1.0.0.tar.gz`

2. Upload these three files to your GitHub Release tag (`v1.0.0`).

3. Install on Linux server in one command:

```bash
REPO=<your-github-user>/<your-repo> VERSION=v1.0.0 bash <(curl -fsSL https://raw.githubusercontent.com/<your-github-user>/<your-repo>/main/deploy/install.sh)
```

After install, CLI commands are available:

```bash
tele-auto status
tele-auto logs
tele-auto restart
tele-auto uninstall
```

`tele-auto uninstall` asks for confirmation. Type `y` to remove:
- systemd service/unit
- install directory (`/opt/tele-auto-go` by default)
- symlinks (`/usr/local/bin/tele-auto` and `~/.local/bin/tele-auto` if created)

## Quick Start

### 1) Backend

```bash
cd backend
cp .env.example .env
# fill required env values
make run-control
```

Control API runs on `http://localhost:3000` by default.

### 2) Frontend

```bash
cd frontend
cp .env.example .env
npm install
npm run dev
```

Frontend runs on `http://localhost:5173` and talks to backend using `VITE_API_BASE_URL`.

## Features in UI

- Onboarding-first setup (required settings -> OTP verify -> dashboard)
- Worker start/stop/restart
- Realtime logs stream
- Simplified settings view (non-essential settings hidden)
- `SOUL.md` read/update

Read backend details in [`backend/README.md`](./backend/README.md).
