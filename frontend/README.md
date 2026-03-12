# tele-auto-go frontend

Vite + React + TypeScript dashboard for controlling the Go backend.

## Setup

```bash
cp .env.example .env
npm install
npm run dev
```

Default frontend URL: `http://localhost:5173`

Set backend URL in `.env`:

```bash
VITE_API_BASE_URL=http://localhost:3000
```

If unset, frontend defaults to current origin (useful in production when backend serves web files).

## Features

- Onboarding-first flow (required settings then OTP verify)
- Worker start/stop/restart controls
- Realtime log stream panel (SSE)
- Simplified settings editor (advanced values hidden)
- SOUL prompt editor

## Build

```bash
npm run build
npm run preview
```
