# Job Scorer Frontend

React + TypeScript dashboard for the Job Scorer pipeline.

## Development

1. Start the Go backend: `cd .. && go run .` (default port 8008)
2. Start the frontend: `npm run dev` (default port 5173)
3. The Vite dev server proxies `/api` and `/health` to the backend

For multi-tenant mode, run backend with:
- `APP_MODE=web` for API + SPA
- `APP_MODE=worker` in a second process for task execution

Auth options:
- Firebase (set `VITE_FIREBASE_*` values)
- Local debug mode (backend `AUTH_BYPASS=true` and use debug email login)

## Production

Build: `npm run build`

For production, set `VITE_API_URL` to your backend URL (e.g. `https://your-service.run.app`) so the frontend can reach the API when served from a different origin.

## Features

- **Dashboard**: KPIs, funnel chart, run status, recent runs
- **Runs**: List runs, trigger new run, view per-stage counts
- **Run Detail**: Pipeline timeline, stage drill-down
- **Job Explorer**: Filter jobs by run and stage, search
- **Settings**: Edit account-scoped policy JSON, cron schedule, notifications, and upload CV
- **Billing**: View credit balance and open Stripe checkout
