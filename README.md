# Job Scorer (Go)

Job Scorer finds jobs from LinkedIn public listings, scores them with an LLM, matches them against your CV, and optionally emails you only the best matches.

## 🚀 What it does

- **Scrape**: query LinkedIn Jobs (public guest endpoints)
- **Prefilter**: policy-based keyword/language/seniority rules
- **Score**: LLM initial screen + CV-based match
- **Notify**: optional HTML email when jobs pass your thresholds
- **Run anywhere**: local binary or Cloud Run + Cloud Scheduler

## ✅ Quick start (local)

1. Install Go \(1.21+\).

2. Create config files:

```bash
cp env.example .env
cp config/config.example.json config/config.json
```

3. Edit `.env` \(secrets + runtime toggles\):

```env
OPENAI_API_KEY=your_openai_api_key_here
OPENAI_MODEL=gpt-4o-mini
POLICY_CONFIG_PATH=config/config.json

# Optional email
SMTP_HOST=smtp.your-provider.com
SMTP_PORT=587
SMTP_SECURE=false
SMTP_USER=your_email@domain.com
SMTP_PASS=your_app_password
SMTP_FROM=your_email@domain.com
SMTP_TO=recipient@domain.com

RUN_ON_STARTUP=true
```

4. Edit `config/config.json` \(your actual policy\):

```json
{
  "app": {
    "cronSchedule": "0 */1 * * *",
    "jobLocations": ["10000000", "20000000"]
  },
  "cv": { "path": "your_cv.pdf" }
}
```

5. Put your CV PDF at the configured path, then run:

```bash
go build -o job-scorer .
./job-scorer
```

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

Tests:

```bash
go test ./...
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

## 🤝 Contributing

PRs welcome. Please add/adjust tests for meaningful changes.

## 📄 License

MIT (see `LICENSE`).