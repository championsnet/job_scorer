#!/usr/bin/env bash
set -euo pipefail

PROJECT_ID="${GOOGLE_CLOUD_PROJECT:-$(gcloud config get-value project 2>/dev/null)}"
REGION="${CLOUD_RUN_REGION:-europe-west1}"
ARTIFACT_REPO="${ARTIFACT_REPO:-job-scorer}"
IMAGE_NAME="${IMAGE_NAME:-job-scorer}"
WEB_SERVICE="${WEB_SERVICE:-job-scorer-web}"
WORKER_SERVICE="${WORKER_SERVICE:-job-scorer-worker}"
TASKS_QUEUE="${CLOUD_TASKS_QUEUE:-job-scorer-runs}"
SCHEDULER_JOB="${SCHEDULER_JOB:-job-scorer-dispatch}"
SCHEDULER_CRON="${SCHEDULER_CRON:-0 */1 * * *}"
SCHEDULER_TIMEZONE="${SCHEDULER_TIMEZONE:-UTC}"
GCS_BUCKET_NAME="${GCS_BUCKET_NAME:-${PROJECT_ID}-job-scorer-data}"
AUTH_BYPASS="${AUTH_BYPASS:-false}"
VITE_API_URL="${VITE_API_URL:-}"
VITE_FIREBASE_API_KEY="${VITE_FIREBASE_API_KEY:-}"
VITE_FIREBASE_AUTH_DOMAIN="${VITE_FIREBASE_AUTH_DOMAIN:-}"
VITE_FIREBASE_PROJECT_ID="${VITE_FIREBASE_PROJECT_ID:-}"
VITE_FIREBASE_APP_ID="${VITE_FIREBASE_APP_ID:-}"
VITE_FIREBASE_MESSAGING_SENDER_ID="${VITE_FIREBASE_MESSAGING_SENDER_ID:-}"

WEB_SA_EMAIL="${WEB_SERVICE_ACCOUNT_EMAIL:-job-scorer-web@${PROJECT_ID}.iam.gserviceaccount.com}"
WORKER_SA_EMAIL="${WORKER_SERVICE_ACCOUNT_EMAIL:-job-scorer-worker@${PROJECT_ID}.iam.gserviceaccount.com}"
SCHEDULER_SA_EMAIL="${SCHEDULER_SERVICE_ACCOUNT_EMAIL:-job-scorer-scheduler@${PROJECT_ID}.iam.gserviceaccount.com}"
TASK_CALLER_SA_EMAIL="${TASK_CALLER_SERVICE_ACCOUNT_EMAIL:-${WEB_SA_EMAIL}}"

if [[ -z "${PROJECT_ID}" ]]; then
  echo "PROJECT_ID is required. Set GOOGLE_CLOUD_PROJECT or gcloud default project."
  exit 1
fi

if [[ "${AUTH_BYPASS}" != "true" ]]; then
  for required in \
    VITE_FIREBASE_API_KEY \
    VITE_FIREBASE_AUTH_DOMAIN \
    VITE_FIREBASE_PROJECT_ID \
    VITE_FIREBASE_APP_ID \
    VITE_FIREBASE_MESSAGING_SENDER_ID; do
    if [[ -z "${!required}" ]]; then
      echo "${required} is required when AUTH_BYPASS=false."
      exit 1
    fi
  done
fi

SHORT_SHA="$(git rev-parse --short HEAD)"
BUILD_TAG="$(date +%Y%m%d-%H%M%S)-${SHORT_SHA}"
IMAGE_URI="${REGION}-docker.pkg.dev/${PROJECT_ID}/${ARTIFACT_REPO}/${IMAGE_NAME}:${BUILD_TAG}"

echo "Building image: ${IMAGE_URI}"
gcloud builds submit \
  --project "${PROJECT_ID}" \
  --config cloudbuild.yaml \
  --substitutions "_IMAGE_URI=${IMAGE_URI},_VITE_API_URL=${VITE_API_URL},_VITE_FIREBASE_API_KEY=${VITE_FIREBASE_API_KEY},_VITE_FIREBASE_AUTH_DOMAIN=${VITE_FIREBASE_AUTH_DOMAIN},_VITE_FIREBASE_PROJECT_ID=${VITE_FIREBASE_PROJECT_ID},_VITE_FIREBASE_APP_ID=${VITE_FIREBASE_APP_ID},_VITE_FIREBASE_MESSAGING_SENDER_ID=${VITE_FIREBASE_MESSAGING_SENDER_ID}"

echo "Deploying worker service..."
gcloud run deploy "${WORKER_SERVICE}" \
  --image "${IMAGE_URI}" \
  --region "${REGION}" \
  --project "${PROJECT_ID}" \
  --service-account "${WORKER_SA_EMAIL}" \
  --no-allow-unauthenticated \
  --memory 2Gi \
  --cpu 2 \
  --max-instances 10 \
  --timeout 3600 \
  --set-env-vars "APP_MODE=worker,GCS_ENABLED=true,GCS_BUCKET_NAME=${GCS_BUCKET_NAME},GCS_PROJECT_ID=${PROJECT_ID},FIRESTORE_PROJECT_ID=${PROJECT_ID},RUN_ON_STARTUP=false,AUTH_BYPASS=${AUTH_BYPASS}" \
  --set-secrets "WORKER_TOKEN=WORKER_TOKEN:latest,OPENAI_API_KEY=OPENAI_API_KEY:latest,SMTP_HOST=SMTP_HOST:latest,SMTP_PORT=SMTP_PORT:latest,SMTP_USER=SMTP_USER:latest,SMTP_PASS=SMTP_PASS:latest,SMTP_FROM=SMTP_FROM:latest,STRIPE_SECRET_KEY=STRIPE_SECRET_KEY:latest,STRIPE_WEBHOOK_SECRET=STRIPE_WEBHOOK_SECRET:latest,FIREBASE_PROJECT_ID=FIREBASE_PROJECT_ID:latest"

WORKER_URL="$(gcloud run services describe "${WORKER_SERVICE}" --region "${REGION}" --project "${PROJECT_ID}" --format='value(status.url)')"
if [[ -z "${WORKER_URL}" ]]; then
  echo "Failed to resolve worker service URL"
  exit 1
fi

echo "Allowing task caller service account to invoke worker..."
gcloud run services add-iam-policy-binding "${WORKER_SERVICE}" \
  --region "${REGION}" \
  --project "${PROJECT_ID}" \
  --member "serviceAccount:${TASK_CALLER_SA_EMAIL}" \
  --role "roles/run.invoker" >/dev/null

echo "Allowing web service account to mint OIDC as task caller..."
gcloud iam service-accounts add-iam-policy-binding "${TASK_CALLER_SA_EMAIL}" \
  --project "${PROJECT_ID}" \
  --member "serviceAccount:${WEB_SA_EMAIL}" \
  --role "roles/iam.serviceAccountUser" >/dev/null

echo "Deploying web service..."
gcloud run deploy "${WEB_SERVICE}" \
  --image "${IMAGE_URI}" \
  --region "${REGION}" \
  --project "${PROJECT_ID}" \
  --service-account "${WEB_SA_EMAIL}" \
  --allow-unauthenticated \
  --memory 1Gi \
  --cpu 1 \
  --max-instances 10 \
  --timeout 300 \
  --set-env-vars "APP_MODE=web,GCS_ENABLED=true,GCS_BUCKET_NAME=${GCS_BUCKET_NAME},GCS_PROJECT_ID=${PROJECT_ID},FIRESTORE_PROJECT_ID=${PROJECT_ID},RUN_ON_STARTUP=false,AUTH_BYPASS=${AUTH_BYPASS},CLOUD_TASKS_PROJECT_ID=${PROJECT_ID},CLOUD_TASKS_LOCATION=${REGION},CLOUD_TASKS_QUEUE=${TASKS_QUEUE},CLOUD_TASKS_WORKER_URL=${WORKER_URL},CLOUD_TASKS_SERVICE_ACCOUNT=${TASK_CALLER_SA_EMAIL}" \
  --set-secrets "WORKER_TOKEN=WORKER_TOKEN:latest,SCHEDULER_TOKEN=SCHEDULER_TOKEN:latest,OPENAI_API_KEY=OPENAI_API_KEY:latest,SMTP_HOST=SMTP_HOST:latest,SMTP_PORT=SMTP_PORT:latest,SMTP_USER=SMTP_USER:latest,SMTP_PASS=SMTP_PASS:latest,SMTP_FROM=SMTP_FROM:latest,STRIPE_SECRET_KEY=STRIPE_SECRET_KEY:latest,STRIPE_WEBHOOK_SECRET=STRIPE_WEBHOOK_SECRET:latest,FIREBASE_PROJECT_ID=FIREBASE_PROJECT_ID:latest"

WEB_URL="$(gcloud run services describe "${WEB_SERVICE}" --region "${REGION}" --project "${PROJECT_ID}" --format='value(status.url)')"
if [[ -z "${WEB_URL}" ]]; then
  echo "Failed to resolve web service URL"
  exit 1
fi

echo "Allowing scheduler service account to invoke web internal dispatch endpoint..."
gcloud run services add-iam-policy-binding "${WEB_SERVICE}" \
  --region "${REGION}" \
  --project "${PROJECT_ID}" \
  --member "serviceAccount:${SCHEDULER_SA_EMAIL}" \
  --role "roles/run.invoker" >/dev/null

SCHEDULER_TOKEN="$(gcloud secrets versions access latest --secret=SCHEDULER_TOKEN --project "${PROJECT_ID}")"

if gcloud scheduler jobs describe "${SCHEDULER_JOB}" --location "${REGION}" --project "${PROJECT_ID}" >/dev/null 2>&1; then
  gcloud scheduler jobs update http "${SCHEDULER_JOB}" \
    --location "${REGION}" \
    --project "${PROJECT_ID}" \
    --schedule "${SCHEDULER_CRON}" \
    --time-zone "${SCHEDULER_TIMEZONE}" \
    --uri "${WEB_URL}/internal/dispatch" \
    --http-method POST \
    --update-headers "X-Scheduler-Token=${SCHEDULER_TOKEN}" \
    --oidc-service-account-email "${SCHEDULER_SA_EMAIL}"
else
  gcloud scheduler jobs create http "${SCHEDULER_JOB}" \
    --location "${REGION}" \
    --project "${PROJECT_ID}" \
    --schedule "${SCHEDULER_CRON}" \
    --time-zone "${SCHEDULER_TIMEZONE}" \
    --uri "${WEB_URL}/internal/dispatch" \
    --http-method POST \
    --headers "X-Scheduler-Token=${SCHEDULER_TOKEN}" \
    --oidc-service-account-email "${SCHEDULER_SA_EMAIL}"
fi

echo "Running smoke tests..."
curl -fsS "${WEB_URL}/health" >/dev/null
WORKER_HEALTH_TOKEN="$(gcloud auth print-identity-token --project "${PROJECT_ID}")"
curl -fsS "${WORKER_URL}/health" \
  -H "Authorization: Bearer ${WORKER_HEALTH_TOKEN}" >/dev/null

echo
echo "Deployment complete."
echo "Web URL:    ${WEB_URL}"
echo "Worker URL: ${WORKER_URL}"
echo "Scheduler job: ${SCHEDULER_JOB} (${SCHEDULER_CRON})"
echo "Image: ${IMAGE_URI}"
