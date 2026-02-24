# Cloud Scheduling Setup Guide

This guide explains how to set up automated job processing using Google Cloud Scheduler + Cloud Run (the proper way to handle cron jobs in the cloud).

## 🔄 How It Works

**Before (Internal Cron - ❌ Doesn't work on Cloud Run):**
```
Container runs cron scheduler → Gets paused when idle → Cron stops working
```

**After (Cloud Scheduler - ✅ Works perfectly):**
```
Google Cloud Scheduler → HTTP POST to /run endpoint → Triggers job processing
```

## 🚀 Quick Deployment

### Option 1: Automated Deployment (Recommended)

Run the deployment script that handles everything:

```bash
# First, make sure you have a .env file with your configuration
cp env.example .env
# Edit .env with your actual values

# Make script executable
chmod +x deploy.sh

# Deploy with default settings (loads .env automatically)
./deploy.sh

# Deploy with custom settings
./deploy.sh --project my-project --service job-scorer --region us-central1 --schedule "0 9 * * *"
```

**Note:** The deployment script automatically loads environment variables from your `.env` file, so make sure it's properly configured before deployment.

### Option 2: Manual Setup

If you prefer to set up step by step:

#### 1. Deploy to Cloud Run

```bash
# Set your project
export PROJECT_ID="your-project-id"

# Deploy the service
gcloud run deploy job-scorer \
    --source . \
    --platform managed \
    --region us-central1 \
    --allow-unauthenticated \
    --memory 1Gi \
    --cpu 1 \
    --max-instances 1 \
    --set-env-vars "GCS_ENABLED=true" \
    --timeout 3600 \
    --project="$PROJECT_ID"
```

#### 2. Get the Service URL

```bash
SERVICE_URL=$(gcloud run services describe job-scorer \
    --platform managed \
    --region us-central1 \
    --format 'value(status.url)' \
    --project="$PROJECT_ID")

echo "Service URL: $SERVICE_URL"
```

#### 3. Create Cloud Scheduler Job

```bash
# Create scheduler job (runs every hour)
gcloud scheduler jobs create http job-scorer-scheduler \
    --location=us-central1 \
    --schedule="0 */1 * * *" \
    --uri="$SERVICE_URL/run" \
    --http-method="POST" \
    --description="Automated job processing for Job Scorer" \
    --project="$PROJECT_ID"
```

## ⚙️ Configuration

### Environment Variables

Set these in Cloud Run:

```bash
# Required
GROQ_API_KEY=your_groq_api_key

# Job search
JOB_LOCATIONS=90009885,90009888
CV_PATH=CV_Vasiliki Ploumistou_22_05.pdf

# Email notifications
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASS=your-app-password
SMTP_FROM=your-email@gmail.com
SMTP_TO=recipient@gmail.com

# Storage (automatically set by deploy script)
GCS_ENABLED=true
GCS_BUCKET_NAME=your-bucket-name
GCS_PROJECT_ID=your-project-id

# Scheduling behavior
RUN_ON_STARTUP=true
CRON_SCHEDULE=0 */1 * * *
```

### Schedule Examples

Common cron schedule patterns:

```bash
# Every hour
--schedule="0 */1 * * *"

# Every day at 9 AM
--schedule="0 9 * * *"

# Every Monday at 9 AM
--schedule="0 9 * * 1"

# Every 30 minutes during business hours (9 AM - 5 PM, weekdays)
--schedule="*/30 9-17 * * 1-5"

# Twice a day (9 AM and 6 PM)
--schedule="0 9,18 * * *"
```

## 📊 Monitoring and Management

### View Service Status

```bash
# Check service health
curl https://your-service-url/health

# View stats dashboard
curl https://your-service-url/stats
```

### Trigger Manual Run

```bash
# Trigger job processing manually
curl -X POST https://your-service-url/run

# View the response
curl -X POST https://your-service-url/run -v
```

### Monitor Scheduler

```bash
# View scheduler job
gcloud scheduler jobs describe job-scorer-scheduler --location=us-central1

# Pause scheduler
gcloud scheduler jobs pause job-scorer-scheduler --location=us-central1

# Resume scheduler
gcloud scheduler jobs resume job-scorer-scheduler --location=us-central1

# Delete scheduler
gcloud scheduler jobs delete job-scorer-scheduler --location=us-central1
```

### View Logs

```bash
# View Cloud Run logs
gcloud run services logs tail job-scorer --region=us-central1

# View recent logs
gcloud logging read "resource.type=cloud_run_revision AND resource.labels.service_name=job-scorer" --limit=50

# Filter by timestamp
gcloud logging read "resource.type=cloud_run_revision AND resource.labels.service_name=job-scorer AND timestamp>='2024-01-15T09:00:00Z'" --limit=20
```

## 🔧 Available Endpoints

Your deployed service provides these endpoints:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Service information and dashboard |
| `/health` | GET | Health check (returns OK) |
| `/run` | POST | Trigger job processing (used by scheduler) |
| `/stats` | GET | Application statistics dashboard |

## 🏗️ Architecture

```
Google Cloud Scheduler
        ↓
    HTTP POST /run
        ↓
   Cloud Run Service
        ↓
  Job Processing Logic
        ↓
  Google Cloud Storage (Data persistence)
        ↓
  Email Notifications
```

## 🔍 Troubleshooting

### Scheduler Not Working

1. **Check scheduler status:**
   ```bash
   gcloud scheduler jobs describe job-scorer-scheduler --location=us-central1
   ```

2. **Verify service URL:**
   ```bash
   gcloud run services list --platform=managed
   ```

3. **Test manually:**
   ```bash
   curl -X POST https://your-service-url/run
   ```

### Service Issues

1. **View logs:**
   ```bash
   gcloud run services logs tail job-scorer --region=us-central1
   ```

2. **Check service health:**
   ```bash
   curl https://your-service-url/health
   ```

3. **Verify environment variables:**
   ```bash
   gcloud run services describe job-scorer --region=us-central1
   ```

### Permission Issues

If you get authentication errors:

```bash
# Create service account for scheduler
gcloud iam service-accounts create job-scorer-scheduler \
    --display-name="Job Scorer Scheduler"

# Grant invoker permission
gcloud run services add-iam-policy-binding job-scorer \
    --member="serviceAccount:job-scorer-scheduler@PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/run.invoker" \
    --region=us-central1

# Update scheduler to use service account
gcloud scheduler jobs update http job-scorer-scheduler \
    --location=us-central1 \
    --oidc-service-account-email="job-scorer-scheduler@PROJECT_ID.iam.gserviceaccount.com"
```

## 💰 Cost Estimation

**Cloud Run:**
- 1 CPU, 1 GB RAM, runs ~5 minutes per hour
- ~$0.50/month for hourly runs

**Cloud Scheduler:**
- 1 job, hourly execution
- ~$0.10/month

**Total: ~$0.60/month** for hourly job processing

## 🔒 Security

- Service account with minimal permissions
- Authenticated scheduler requests
- Environment variables for secrets
- No public cron endpoints exposed

## 📈 Scaling

- **Frequency**: Adjust `--schedule` parameter
- **Concurrency**: Modify `--max-instances` 
- **Resources**: Increase `--memory` and `--cpu`
- **Timeout**: Set `--timeout` for long-running jobs

## ✅ Best Practices

1. **Use Cloud Scheduler** instead of internal cron for Cloud Run
2. **Set appropriate timeouts** for job processing
3. **Monitor execution** through Cloud Logging
4. **Use service accounts** for authentication
5. **Set memory/CPU limits** based on job requirements
6. **Enable error notifications** for failed executions

## 🎯 Next Steps

After deployment:

1. Test manual trigger: `curl -X POST https://your-service-url/run`
2. Check the `/stats` dashboard for monitoring
3. Verify scheduler in Google Cloud Console
4. Monitor first few scheduled executions
5. Adjust schedule frequency as needed

The scheduler will now reliably trigger your job processing according to your cron schedule, without any container lifecycle issues! 