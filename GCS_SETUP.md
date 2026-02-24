# Google Cloud Storage Integration

This document explains how to set up and use Google Cloud Storage (GCS) with the Job Scorer application for cloud-based data persistence.

## Overview

The Job Scorer application now supports storing all data in Google Cloud Storage instead of local files. This includes:

- **Job Data**: `allJobs.json`, `promisingJobs.json`, `finalEvaluatedJobs.json`
- **Processed Job IDs**: `processed_job_ids.json` to avoid duplicate processing
- **Checkpoints**: All pipeline stage checkpoints and daily snapshots
- **Log Files**: Application logs are automatically uploaded to GCS

## Benefits of GCS Integration

1. **Cloud-Native**: Perfect for deployments on Google Cloud Run or other cloud platforms
2. **Persistence**: Data survives container restarts and redeployments
3. **Scalability**: No local storage limits
4. **Backup**: Built-in redundancy and durability
5. **Accessibility**: Access your data from anywhere
6. **Fallback Support**: Gracefully falls back to local storage if GCS is unavailable

## Setup Instructions

### 1. Create a Google Cloud Storage Bucket

```bash
# Create a new bucket (replace with your unique bucket name)
gsutil mb gs://your-job-scorer-bucket

# Set bucket permissions (optional - adjust as needed)
gsutil iam ch serviceAccount:your-service-account@your-project.iam.gserviceaccount.com:roles/storage.admin gs://your-job-scorer-bucket
```

### 2. Set Up Authentication

#### Option A: Service Account (Recommended for Production)

1. Create a service account in Google Cloud Console
2. Grant it `Storage Admin` or `Storage Object Admin` role
3. Download the service account key file
4. Set the environment variable:
   ```bash
   export GOOGLE_APPLICATION_CREDENTIALS="/path/to/your/service-account-key.json"
   ```

#### Option B: Application Default Credentials (Development)

```bash
# Install and authenticate with gcloud CLI
gcloud auth application-default login
```

### 3. Configure Environment Variables

Add these variables to your `.env` file:

```bash
# Google Cloud Storage Configuration
GCS_ENABLED=true
GCS_BUCKET_NAME=your-job-scorer-bucket
GCS_PROJECT_ID=your-gcp-project-id
GCS_FALLBACK_DIR=gcs_fallback
```

## Configuration Options

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `GCS_ENABLED` | Enable/disable GCS storage | `false` | No |
| `GCS_BUCKET_NAME` | Name of your GCS bucket | - | Yes (if enabled) |
| `GCS_PROJECT_ID` | Your Google Cloud Project ID | - | Yes (if enabled) |
| `GCS_FALLBACK_DIR` | Local directory to use if GCS fails | `gcs_fallback` | No |

## GCS Storage Structure

Your bucket will be organized as follows:

```
your-bucket/
├── job-data/
│   ├── allJobs.json
│   ├── promisingJobs.json
│   └── finalEvaluatedJobs.json
├── data/
│   └── processed_job_ids.json
├── checkpoints/
│   └── 2024-01-15_14-30-00/
│       ├── checkpoint_all_jobs_14-30-15.json
│       ├── checkpoint_prefiltered_14-31-02.json
│       ├── checkpoint_evaluated_14-32-45.json
│       ├── checkpoint_promising_14-35-12.json
│       ├── checkpoint_final_evaluated_14-38-30.json
│       ├── checkpoint_notification_14-40-15.json
│       └── daily_snapshot.json
└── logs/
    ├── JobScorer_2024-01-15_14-30-00.log
    └── CheckpointService_2024-01-15_14-30-00.log
```

## How It Works

### Automatic Fallback

If GCS is enabled but unavailable (network issues, authentication problems, etc.), the application automatically falls back to local storage without failing. You'll see warning messages in the logs.

### Local + Cloud Hybrid

- Logs are written locally first, then uploaded to GCS
- Job processing data is written directly to GCS
- If GCS fails during runtime, data is saved to the fallback directory

### Seamless Migration

Existing local data is not automatically migrated. To migrate:

1. Enable GCS storage
2. The application will start fresh (processed job IDs will be empty)
3. Historical data remains in local files

## Usage Examples

### Viewing Your Data

```bash
# List all files in your bucket
gsutil ls -r gs://your-job-scorer-bucket/

# Download a specific checkpoint
gsutil cp gs://your-job-scorer-bucket/checkpoints/2024-01-15_14-30-00/daily_snapshot.json ./

# Download all job data
gsutil -m cp -r gs://your-job-scorer-bucket/job-data/ ./local-backup/
```

### Monitoring Storage

```bash
# Check bucket size and object count
gsutil du -s gs://your-job-scorer-bucket/

# List recent log files
gsutil ls gs://your-job-scorer-bucket/logs/ | tail -5
```

## Cost Considerations

GCS pricing is very reasonable for typical job scorer usage:

- **Storage**: ~$0.20 per GB per month (Standard storage)
- **Operations**: ~$0.05 per 10,000 operations
- **Network**: Egress charges apply for downloads

Estimated monthly costs for typical usage:
- Data: ~10 MB → **$0.002/month**
- Operations: ~1,000/month → **$0.005/month**
- **Total: ~$0.01/month** (practically free)

## Deployment on Google Cloud Run

When deploying to Cloud Run, GCS integration is highly recommended:

```bash
# Build and deploy with GCS enabled
gcloud run deploy job-scorer \
  --source . \
  --set-env-vars="GCS_ENABLED=true,GCS_BUCKET_NAME=your-bucket,GCS_PROJECT_ID=your-project" \
  --region=us-central1
```

The Cloud Run service will automatically have permissions to access GCS in the same project.

## Troubleshooting

### Common Issues

1. **Authentication Errors**
   ```
   Failed to create GCS client: oauth2: cannot fetch token
   ```
   **Solution**: Set up proper authentication (service account or ADC)

2. **Bucket Access Denied**
   ```
   Failed to access GCS bucket 'your-bucket': storage: bucket doesn't exist
   ```
   **Solution**: Verify bucket name and ensure it exists

3. **Fallback Mode**
   ```
   GCS storage disabled, using local fallback directory: gcs_fallback
   ```
   **Solution**: This is normal - check GCS configuration if you want cloud storage

### Debugging

Enable debug logging to see detailed GCS operations:

```bash
export LOG_LEVEL=DEBUG
```

### Testing GCS Connection

You can test your GCS setup:

```bash
# Test bucket access
gsutil ls gs://your-job-scorer-bucket/

# Test authentication
gcloud auth list
```

## Security Best Practices

1. **Use IAM Roles**: Grant minimal necessary permissions
2. **Service Accounts**: Use dedicated service accounts for production
3. **VPC Integration**: Consider VPC Service Controls for enterprise deployments
4. **Encryption**: GCS provides encryption at rest by default
5. **Access Logging**: Enable Cloud Audit Logs for compliance

## Monitoring and Alerts

Set up monitoring for your GCS usage:

1. **Cloud Monitoring**: Monitor storage usage and operation counts
2. **Log-based Metrics**: Create metrics from application logs
3. **Alerts**: Set up alerts for authentication failures or high costs

## Migration and Backup

### Backup Strategy

- GCS provides 99.999999999% (11 9's) durability
- Consider versioning for critical data:
  ```bash
  gsutil versioning set on gs://your-job-scorer-bucket
  ```

### Cross-Region Replication

For higher availability:
```bash
# Create multi-region bucket
gsutil mb -c multi_regional -l us gs://your-job-scorer-backup
```

This completes the GCS integration setup. Your Job Scorer application is now cloud-ready! 