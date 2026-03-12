# tele-auto-go backend

Go backend for Telegram native-account AI auto-reply with a control API.

The frontend now uses onboarding-first flow:
1. save required settings
2. request OTP and verify Telegram login
3. open main dashboard

Backend can serve built frontend static files from `WEB_DIR` (default `./web`).

## What Runs

- Telegram worker (auto-reply engine)
- HTTP control API for frontend dashboard

## Setup

```bash
cp .env.example .env
```

Required values:

- `TG_API_ID`
- `TG_API_HASH`
- `OPENAI_BASE_URL`
- `OPENAI_API_KEY`
- `OPENAI_MODEL`

Optional values:
- `TG_SESSION_FILE` (default `./data/session.json`)
- `SQLITE_PATH` (default `./data/app.db`)
- `SOUL_PROMPT_PATH` (default `./SOUL.md`)

## Run

Recommended mode (control API + managed worker):

```bash
make run-control
```

Legacy worker-only mode:

```bash
make run
```

Default API address: `http://localhost:3000`

## Main API Endpoints

- `GET /health`
- `GET /api/auth/status`
- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/service/status`
- `POST /api/service/start`
- `POST /api/service/stop`
- `POST /api/service/restart`
- `GET /api/settings`
- `PUT /api/settings`
- `GET /api/soul`
- `PUT /api/soul`
- `GET /api/logs`
- `GET /api/logs/stream` (SSE)

## Login Notes

`POST /api/auth/login` expects:

```json
{
  "phone": "+15551234567",
  "code": "12345",
  "password": "optional-2fa"
}
```

If `code` is omitted, API requests OTP and stores `phone_code_hash` in memory for verify step.
Telegram often delivers OTP to the Telegram app first (not SMS).

## Fixed Defaults (Hardcoded)

The following are fixed at application level and no longer editable in UI settings:
- context limit = `20`
- max tokens = `320`
- private-only mode = `true`
- ignore bots/groups/media-only = `true`
- log context = `false`
- human delay = `3000..10000` ms

## Build

```bash
make build
make build-control
```

## Environment Extras

- `CONTROL_PORT` (optional): control API port override
- `FRONTEND_ORIGIN` (optional): CORS origin (default `http://localhost:5173`)
- `WEB_DIR` (optional): static frontend dir to serve (default `./web`)

## Production CLI

Installer creates `tele-auto` command:
- `tele-auto status`
- `tele-auto logs`
- `tele-auto restart`
- `tele-auto uninstall` (requires confirmation `y`)
