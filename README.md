# Job Scorer (Go)

Job Scorer finds jobs from LinkedIn public listings, scores them with an LLM, matches them against your CV, and optionally emails you only the best matches. It works for **any** field, seniority, location, and language — you tell it what you want in a setup wizard; nothing is hardcoded.

**Two ways to run it:**
- **Local / self-host (recommended for individuals):** download the app, open a browser, fill in a short setup wizard. No config files, no Go, no coding. This is what most people want — see the Quick start below.
- **Multi-tenant SaaS mode** (`APP_MODE=web|worker`): Firestore + Cloud Tasks + Firebase auth + per-account settings/CVs/runs/credits. See [Multi-tenant mode](#-multi-tenant-mode-new).

## 🚀 What it does

- **Scrape**: query LinkedIn Jobs (public guest endpoints)
- **Prefilter**: policy-based keyword/language/seniority rules
- **Score**: LLM initial screen + CV-based match
- **Notify**: optional HTML email when jobs pass your thresholds
- **Run anywhere**: local binary or Cloud Run + Cloud Scheduler

## ✅ Quick start (local) — the setup wizard

You need two things: an **OpenAI API key** ([get one here](https://platform.openai.com/api-keys)) and, optionally, an email account for alerts. Everything else is handled by the wizard.

### Option A — Download the app (no coding)

1. Download your platform's app from the [Releases page](../../releases):
   - **macOS:** `…_macos-apple-silicon.zip` (M1/M2/M3) or `…_macos-intel.zip`
   - **Windows:** `…_windows-x86_64.zip`
   - **Linux:** `…_linux-x86_64.tar.gz`
2. Unzip and open it — **a Job Scorer window opens** (no browser, no terminal):
   - **macOS:** double-click **Job Scorer.app**. The first time, right-click → **Open** (it's unsigned).
   - **Windows:** double-click **Job Scorer.exe**.
   - **Linux:** run `./job-scorer` (needs `libwebkit2gtk-4.0`).
3. The **setup wizard** walks you through it, right in the window:
   - Paste your OpenAI key (there's a **Test key** button).
   - Pick where you want to work — search any city, or one-click a **Switzerland** metro area. No geoId hunting.
   - Choose roles/fields, seniority, and the languages you speak (pick-lists, plus "type your own").
   - Upload your CV (PDF, `.txt`, or `.md`).
   - Optionally turn on email alerts (Gmail/Outlook/iCloud presets + a **Send test email** button).
4. Click **Save & start**. It scores now, keeps checking on your schedule, and shows results in the **dashboard** inside the app. Reopen setup anytime from the **Reconfigure** button.

Settings are saved in your user data folder (macOS: `~/Library/Application Support/JobScorer`).

### Option B — Docker (one command)

```bash
docker compose up
```

Then open **http://localhost:8008** and complete the wizard. Everything is saved in `./jobscorer-data/`.

### Option C — Build from source (for developers)

Native desktop app (macOS — opens a real window):

```bash
./scripts/build_macos_app.sh          # produces dist/Job Scorer.app
open "dist/Job Scorer.app"
```

Or the headless / browser version (any OS, pure Go, no CGO — opens in your browser):

```bash
go build -o job-scorer .
./job-scorer                          # auto-opens http://localhost:8008
```

Desktop builds use `-tags desktop` and need CGO + the OS webview (WebKit on macOS,
WebView2 on Windows, webkit2gtk on Linux). The default `go build` is headless and
cross-compiles trivially — it's what Docker and servers use.

Advanced users can skip the wizard and edit `config/config.json` directly (copy `config/config.example.json`). Every prompt/threshold is configurable there.

## 🧱 Multi-tenant mode (new)

Set `APP_MODE` and run one or two services:

```bash
# Web/API + SPA
APP_MODE=web go run .

# Worker (in another terminal)
APP_MODE=worker go run .
```

Required env for multi-tenant mode:
- Firebase config (`FIREBASE_PROJECT_ID`, optional `FIREBASE_CREDENTIALS_FILE`)
- Cloud Tasks config for web mode (`CLOUD_TASKS_*`)
- `WORKER_TOKEN`, `SCHEDULER_TOKEN`

Development shortcut:
- set `AUTH_BYPASS=true`
- login from the frontend with a debug email (the client sends `Authorization: Bearer dev:<email>`)

## ⚙️ Configuration (the important bits)

### Job locations (LinkedIn geo IDs)

Locations live in `config/config.json` as `app.jobLocations`:

```json
{ "app": { "jobLocations": ["10000000", "20000000"] } }
```

#### How to get a LinkedIn `geoId`

- **Method A (fastest): from the URL**
  - Open LinkedIn → **Jobs**
  - Set a **Location** filter (city/region)
  - Copy the browser URL and look for `geoId=...`
  - Example: if the URL contains `geoId=102890719`, then your geo ID is `102890719`

- **Method B: from network requests (more reliable if URL changes)**
  - Open DevTools → Network tab
  - Load/scroll a LinkedIn Jobs search results page
  - Filter for a request containing `seeMoreJobPostings` (LinkedIn jobs-guest API)
  - Open the request URL and copy the `geoId` query parameter

Notes:
- `jobLocations` is an array so you can target multiple regions.
- `JOB_LOCATIONS` env var still exists as a **deprecated fallback** for backwards compatibility, but config JSON is the intended source of truth.

### CV path

Set in `config/config.json`:

```json
{ "cv": { "path": "your_cv.pdf" } }
```

You can provide either a PDF (`.pdf`) or a text CV file (`.md`, `.markdown`, `.txt`).

### Scheduling

- **Local**: the app starts an HTTP server and can run on startup.
- **Cloud Run**: use **Cloud Scheduler** to call `POST /run` (see `CLOUD_SCHEDULING.md`).

### Provider / API

- Uses `OPENAI_API_KEY` and an OpenAI-compatible endpoint (`OPENAI_BASE_URL`).
- `GROQ_API_KEY` is supported as a legacy fallback.

## 🔧 Development

Hot reload + helpers:

```bash
./dev.sh setup
./dev.sh run
```

### Frontend Dashboard

A separate React dashboard is in `frontend/`. To run it:

```bash
# Terminal 1: start the Go backend
go run .

# Terminal 2: start the frontend (proxies /api to backend)
cd frontend && npm run dev
```

Open http://localhost:5173 for the dashboard (runs, analytics, job explorer).

The frontend now includes:
- auth (Firebase or local debug-bypass)
- account settings editor for policy JSON + schedule + notification emails
- CV upload
- billing/credits page

Tests:

```bash
go test ./...
cd frontend && npm run test
```

## 📁 Outputs

By default it writes JSON artifacts like:
- `allJobs.json`
- `promisingJobs.json`
- `finalEvaluatedJobs.json`

## 💰 Cost estimate (gpt-4o-mini)

Using gpt-4o-mini pricing (input **$0.15 / 1M**, cached input **$0.075 / 1M**, output **$0.60 / 1M**; see OpenAI’s [GPT-4o mini pricing note](https://developers.openai.com/api/docs/pricing)):

- **Average LLM cost per 100 jobs searched**: **~$0.0023**
- **Estimated LLM cost if run hourly for one month** (≈720 runs): **~$3.11/month**

These are ballpark estimates; actual cost varies with CV length, job description length, and how many jobs reach the LLM evaluation stages.

## 🚨 Troubleshooting (common)

- **No jobs**: confirm your `app.jobLocations` geo IDs are valid; start with a single geo ID and a small `MAX_JOBS_PER_LOCATION`.
- **LinkedIn throttling**: reduce `MAX_JOBS_PER_LOCATION`, increase delays in policy scraper settings, and run less frequently.
- **CV parse fails**: verify the PDF path; try simpler PDFs; check logs for which parser was used.
- **No emails**: SMTP is optional; leave SMTP vars empty to disable notifications.

## 🔒 Security & legal

- Keep secrets in `.env` or your cloud secret manager; never commit real keys.
- Don’t commit your CV PDF or generated outputs.
- Ensure your usage complies with LinkedIn’s terms and local regulations.

## ☁️ GCP scripts

Two scripts are provided for the new deploy path:
- `scripts/bootstrap_gcp.sh` (one-time infra/bootstrap)
- `scripts/deploy_release.sh` (build, deploy worker+web, wire scheduler, smoke tests)

Typical flow:

```bash
./scripts/bootstrap_gcp.sh
./scripts/deploy_release.sh
```

The release script deploys:
- **worker** (`APP_MODE=worker`) for Cloud Tasks execution
- **web** (`APP_MODE=web`) for SPA + API + scheduler dispatch endpoint

## ✅ Deploy from current state

This is the exact sequence to deploy **from this repo state today**.

### 0) Prerequisites

- `gcloud` authenticated and project selected:

```bash
gcloud auth login
gcloud config set project YOUR_PROJECT_ID
```

- Billing enabled on project.
- A region chosen (defaults to `europe-west1` in scripts).

### 1) Run one-time bootstrap

```bash
./scripts/bootstrap_gcp.sh
```

This creates:
- Artifact Registry repo
- Firestore database (native mode)
- Cloud Storage bucket
- Cloud Tasks queue
- Service accounts
- Base Secret Manager secrets

### 2) Replace placeholder secrets

Bootstrap seeds several secrets as `replace-me`. Update them before deploy:

```bash
printf "%s" "YOUR_OPENAI_KEY" | gcloud secrets versions add OPENAI_API_KEY --data-file=-
printf "%s" "smtp.your-provider.com" | gcloud secrets versions add SMTP_HOST --data-file=-
printf "%s" "587" | gcloud secrets versions add SMTP_PORT --data-file=-
printf "%s" "YOUR_SMTP_USER" | gcloud secrets versions add SMTP_USER --data-file=-
printf "%s" "YOUR_SMTP_PASS" | gcloud secrets versions add SMTP_PASS --data-file=-
printf "%s" "no-reply@your-domain.com" | gcloud secrets versions add SMTP_FROM --data-file=-
printf "%s" "YOUR_STRIPE_SECRET_KEY" | gcloud secrets versions add STRIPE_SECRET_KEY --data-file=-
printf "%s" "YOUR_STRIPE_WEBHOOK_SECRET" | gcloud secrets versions add STRIPE_WEBHOOK_SECRET --data-file=-
printf "%s" "YOUR_FIREBASE_PROJECT_ID" | gcloud secrets versions add FIREBASE_PROJECT_ID --data-file=-
```

### 3) Configure Firebase auth for production

Enable Firebase Auth providers (Google and/or Email/Password) in Firebase Console, then export the frontend runtime config so it is injected at image build time:

```bash
export AUTH_BYPASS=false
export VITE_FIREBASE_API_KEY="..."
export VITE_FIREBASE_AUTH_DOMAIN="YOUR_PROJECT.firebaseapp.com"
export VITE_FIREBASE_PROJECT_ID="YOUR_PROJECT"
export VITE_FIREBASE_APP_ID="..."
export VITE_FIREBASE_MESSAGING_SENDER_ID="..."
# Optional (defaults to same-origin /api)
export VITE_API_URL=""
```

Also add your Cloud Run web domain to Firebase Auth authorized domains after the first deploy URL is known.

### 4) Deploy web + worker

```bash
./scripts/deploy_release.sh
```

Optional overrides:

```bash
export CLOUD_RUN_REGION=us-central1
export SCHEDULER_CRON="0 */2 * * *"
export SCHEDULER_TIMEZONE="Europe/Athens"
./scripts/deploy_release.sh
```

### 5) Verify deployment

From script output, copy `WEB_URL` and `WORKER_URL`:

```bash
curl -fsS "$WEB_URL/health"
WORKER_HEALTH_TOKEN="$(gcloud auth print-identity-token)"
curl -fsS "$WORKER_URL/health" -H "Authorization: Bearer ${WORKER_HEALTH_TOKEN}"
```

Open `WEB_URL` in browser.

If `AUTH_BYPASS=false`:
- Sign in using enabled Firebase provider(s).
- Verify `/api/v1/me` succeeds after login (open app pages that call it).
- Go to **Settings**:
  - upload CV
  - set cron / timezone
  - set notification recipient emails
  - save policy JSON
- Go to **Billing** and add credits via Stripe (or seed credits manually in DB for testing).
- Trigger a run from **Runs** page.

### 6) Env/secret map actually used in Cloud Run

**Env vars (non-secret):**
- `APP_MODE` (`web` or `worker`)
- `GCS_ENABLED` (`true`)
- `GCS_BUCKET_NAME`
- `GCS_PROJECT_ID`
- `RUN_ON_STARTUP` (`false`)
- `AUTH_BYPASS` (`true` or `false`)
- `CLOUD_TASKS_PROJECT_ID` (web)
- `CLOUD_TASKS_LOCATION` (web)
- `CLOUD_TASKS_QUEUE` (web)
- `CLOUD_TASKS_WORKER_URL` (web)
- `CLOUD_TASKS_SERVICE_ACCOUNT` (web)

**Secrets:**
- `WORKER_TOKEN`
- `SCHEDULER_TOKEN` (web)
- `OPENAI_API_KEY`
- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USER`
- `SMTP_PASS`
- `SMTP_FROM`
- `STRIPE_SECRET_KEY`
- `STRIPE_WEBHOOK_SECRET`
- `FIREBASE_PROJECT_ID`

### 6.5) Firestore composite indexes (required)

If your project has no composite indexes yet, analytics and scheduler dispatch endpoints can return `500`.
Create these once per project:

```bash
gcloud firestore indexes composite create \
  --project "$GOOGLE_CLOUD_PROJECT" \
  --collection-group="runs" \
  --query-scope="COLLECTION" \
  --field-config="field-path=account_id,order=ASCENDING" \
  --field-config="field-path=created_at,order=DESCENDING"

gcloud firestore indexes composite create \
  --project "$GOOGLE_CLOUD_PROJECT" \
  --collection-group="accounts" \
  --query-scope="COLLECTION" \
  --field-config="field-path=schedule_enabled,order=ASCENDING" \
  --field-config="field-path=next_run_at,order=ASCENDING"
```

### 7) Daily ops commands

```bash
gcloud run services logs tail job-scorer-web --region="$CLOUD_RUN_REGION"
gcloud run services logs tail job-scorer-worker --region="$CLOUD_RUN_REGION"
gcloud scheduler jobs describe job-scorer-dispatch --location="$CLOUD_RUN_REGION"
```

## 🤝 Contributing

PRs welcome. Please add/adjust tests for meaningful changes.

## 📄 License

MIT (see `LICENSE`).