#!/usr/bin/env bash
set -euo pipefail

PROJECT_ID="${GOOGLE_CLOUD_PROJECT:-$(gcloud config get-value project 2>/dev/null)}"
REGION="${CLOUD_RUN_REGION:-europe-west1}"
ARTIFACT_REPO="${ARTIFACT_REPO:-job-scorer}"
BUCKET_NAME="${GCS_BUCKET_NAME:-${PROJECT_ID}-job-scorer-data}"
TASKS_QUEUE="${CLOUD_TASKS_QUEUE:-job-scorer-runs}"

WEB_SA="${WEB_SERVICE_ACCOUNT:-job-scorer-web}"
WORKER_SA="${WORKER_SERVICE_ACCOUNT:-job-scorer-worker}"
SCHEDULER_SA="${SCHEDULER_SERVICE_ACCOUNT:-job-scorer-scheduler}"

if [[ -z "${PROJECT_ID}" ]]; then
  echo "PROJECT_ID is required. Set GOOGLE_CLOUD_PROJECT or gcloud default project."
  exit 1
fi

echo "Using project: ${PROJECT_ID}"
echo "Using region: ${REGION}"

gcloud services enable \
  run.googleapis.com \
  cloudbuild.googleapis.com \
  artifactregistry.googleapis.com \
  firestore.googleapis.com \
  cloudscheduler.googleapis.com \
  cloudtasks.googleapis.com \
  secretmanager.googleapis.com \
  iam.googleapis.com \
  firebase.googleapis.com \
  identitytoolkit.googleapis.com \
  --project "${PROJECT_ID}"

if ! gcloud artifacts repositories describe "${ARTIFACT_REPO}" --location="${REGION}" --project="${PROJECT_ID}" >/dev/null 2>&1; then
  gcloud artifacts repositories create "${ARTIFACT_REPO}" \
    --repository-format=docker \
    --location="${REGION}" \
    --description="Job Scorer container images" \
    --project="${PROJECT_ID}"
fi

for SA in "${WEB_SA}" "${WORKER_SA}" "${SCHEDULER_SA}"; do
  SA_EMAIL="${SA}@${PROJECT_ID}.iam.gserviceaccount.com"
  if ! gcloud iam service-accounts describe "${SA_EMAIL}" --project="${PROJECT_ID}" >/dev/null 2>&1; then
    gcloud iam service-accounts create "${SA}" --project="${PROJECT_ID}"
  fi
done

echo "Ensuring Firestore database exists (native mode)..."
if ! gcloud firestore databases describe --database="(default)" --project="${PROJECT_ID}" >/dev/null 2>&1; then
  gcloud firestore databases create --database="(default)" --location="${REGION}" --type=firestore-native --project="${PROJECT_ID}"
else
  DB_TYPE="$(gcloud firestore databases describe --database="(default)" --project="${PROJECT_ID}" --format='value(type)')"
  if [[ "${DB_TYPE}" != "FIRESTORE_NATIVE" ]]; then
    echo "ERROR: Firestore database '(default)' in project ${PROJECT_ID} is ${DB_TYPE}, but this app requires FIRESTORE_NATIVE."
    echo "Create a new project with Firestore Native mode, or recreate the default database as Firestore Native."
    exit 1
  fi
fi

if ! gcloud storage buckets describe "gs://${BUCKET_NAME}" --project="${PROJECT_ID}" >/dev/null 2>&1; then
  gcloud storage buckets create "gs://${BUCKET_NAME}" --location="${REGION}" --project="${PROJECT_ID}"
fi

if ! gcloud tasks queues describe "${TASKS_QUEUE}" --location="${REGION}" --project="${PROJECT_ID}" >/dev/null 2>&1; then
  gcloud tasks queues create "${TASKS_QUEUE}" --location="${REGION}" --project="${PROJECT_ID}"
fi

create_secret_if_missing() {
  local secret_name="$1"
  local secret_value="$2"
  if ! gcloud secrets describe "${secret_name}" --project "${PROJECT_ID}" >/dev/null 2>&1; then
    printf "%s" "${secret_value}" | gcloud secrets create "${secret_name}" \
      --data-file=- \
      --replication-policy=automatic \
      --project="${PROJECT_ID}"
  fi
}

create_secret_if_missing "WORKER_TOKEN" "$(openssl rand -hex 32)"
create_secret_if_missing "SCHEDULER_TOKEN" "$(openssl rand -hex 32)"
create_secret_if_missing "OPENAI_API_KEY" "replace-me"
create_secret_if_missing "STRIPE_SECRET_KEY" "replace-me"
create_secret_if_missing "STRIPE_WEBHOOK_SECRET" "replace-me"
create_secret_if_missing "SMTP_HOST" "replace-me"
create_secret_if_missing "SMTP_PORT" "587"
create_secret_if_missing "SMTP_USER" "replace-me"
create_secret_if_missing "SMTP_PASS" "replace-me"
create_secret_if_missing "SMTP_FROM" "replace-me@example.com"
create_secret_if_missing "FIREBASE_PROJECT_ID" "${PROJECT_ID}"

WEB_SA_EMAIL="${WEB_SA}@${PROJECT_ID}.iam.gserviceaccount.com"
WORKER_SA_EMAIL="${WORKER_SA}@${PROJECT_ID}.iam.gserviceaccount.com"
SCHEDULER_SA_EMAIL="${SCHEDULER_SA}@${PROJECT_ID}.iam.gserviceaccount.com"
TASK_CALLER_SA_EMAIL="${TASK_CALLER_SERVICE_ACCOUNT_EMAIL:-${WEB_SA_EMAIL}}"

gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${WEB_SA_EMAIL}" \
  --role="roles/cloudtasks.enqueuer" >/dev/null
gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${WEB_SA_EMAIL}" \
  --role="roles/datastore.user" >/dev/null
gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${WEB_SA_EMAIL}" \
  --role="roles/secretmanager.secretAccessor" >/dev/null
gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${WEB_SA_EMAIL}" \
  --role="roles/storage.admin" >/dev/null
gcloud iam service-accounts add-iam-policy-binding "${TASK_CALLER_SA_EMAIL}" \
  --project "${PROJECT_ID}" \
  --member="serviceAccount:${WEB_SA_EMAIL}" \
  --role="roles/iam.serviceAccountUser" >/dev/null

gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${WORKER_SA_EMAIL}" \
  --role="roles/datastore.user" >/dev/null
gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${WORKER_SA_EMAIL}" \
  --role="roles/secretmanager.secretAccessor" >/dev/null
gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${WORKER_SA_EMAIL}" \
  --role="roles/storage.admin" >/dev/null

echo "Bootstrap complete."
echo "Project: ${PROJECT_ID}"
echo "Region: ${REGION}"
echo "Firestore database: (default)"
echo "Bucket: gs://${BUCKET_NAME}"
echo "Tasks queue: ${TASKS_QUEUE}"
echo "Service accounts:"
echo "  - ${WEB_SA_EMAIL}"
echo "  - ${WORKER_SA_EMAIL}"
echo "  - ${SCHEDULER_SA_EMAIL}"
echo
echo "Next: run scripts/deploy_release.sh to deploy web + worker services."
