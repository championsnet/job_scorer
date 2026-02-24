#!/bin/bash
# deploy.sh

# Job Scorer Deployment Script for Google Cloud Run + Scheduler
# This script deploys the application and sets up automated scheduling

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_ID="${GOOGLE_CLOUD_PROJECT}"
SERVICE_NAME="${CLOUD_RUN_SERVICE:-job-scorer}"
REGION="${CLOUD_RUN_REGION:-europe-west1}"
MEMORY="${CLOUD_RUN_MEMORY:-1Gi}"
CPU="${CLOUD_RUN_CPU:-1}"
MAX_INSTANCES="${CLOUD_RUN_MAX_INSTANCES:-1}"
SCHEDULER_JOB_NAME="${SERVICE_NAME}-scheduler"
CRON_SCHEDULE="${CRON_SCHEDULE:-0 */1 * * *}"

print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_prerequisites() {
    print_status "Checking prerequisites..."
    
    # Check if gcloud is installed
    if ! command -v gcloud &> /dev/null; then
        print_error "gcloud CLI is not installed. Please install it first."
        exit 1
    fi
    
    # Check if authenticated
    if ! gcloud auth list --filter=status:ACTIVE --format="value(account)" | head -n1 > /dev/null; then
        print_error "Not authenticated with gcloud. Run: gcloud auth login"
        exit 1
    fi
    
    # Check project ID
    if [ -z "$PROJECT_ID" ]; then
        PROJECT_ID=$(gcloud config get-value project)
        if [ -z "$PROJECT_ID" ]; then
            print_error "No project ID set. Set GOOGLE_CLOUD_PROJECT or run: gcloud config set project YOUR_PROJECT_ID"
            exit 1
        fi
    fi
    
    print_success "Prerequisites check passed"
    print_status "Project ID: $PROJECT_ID"
    print_status "Service: $SERVICE_NAME"
    print_status "Region: $REGION"
}

enable_apis() {
    print_status "Enabling required Google Cloud APIs..."
    
    gcloud services enable \
        cloudbuild.googleapis.com \
        run.googleapis.com \
        cloudscheduler.googleapis.com \
        storage-api.googleapis.com \
        --project="$PROJECT_ID"
    
    print_success "APIs enabled"
}

cleanup_build_artifacts() {
    print_status "Cleaning up build artifacts..."
    
    # Remove any local build artifacts that might interfere
    rm -f job-scorer
    rm -rf tmp/
    rm -f .dockerignore
    
    # Create fresh .dockerignore to exclude unnecessary files
    cat > .dockerignore << 'EOF'
.git
.gitignore
README.md
SETUP.md
TESTING.md
GCS_SETUP.md
CLOUD_SCHEDULING.md
deploy.sh
dev.sh
test.sh
*.log
coverage/
data/logs/
data/checkpoints/
tmp/
.env.example
EOF
    
    print_success "Build artifacts cleaned"
}

build_and_deploy() {
    print_status "Building and deploying to Cloud Run..."
    
    # Clean up first to avoid any stale artifacts
    cleanup_build_artifacts
    
    # Generate build timestamp and git hash for cache busting
    BUILD_TIMESTAMP=$(date +%Y%m%d-%H%M%S)
    GIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    BUILD_TAG="${BUILD_TIMESTAMP}-${GIT_HASH}"
    
    print_status "Build tag: $BUILD_TAG"
    
    # Prepare environment variables including build info
    ENV_VARS="^##^GCS_ENABLED=true##BUILD_TAG=$BUILD_TAG"
    
    # Check if .env file exists and add variables from it
    if [ -f ".env" ]; then
        print_status "Loading environment variables from .env file..."
        
        # Convert .env to Cloud Run format, skipping comments and empty lines
        while IFS='=' read -r key value; do
            # Skip comments and empty lines
            if [[ ! "$key" =~ ^[[:space:]]*# ]] && [[ -n "$key" ]] && [[ -n "$value" ]]; then
                # Remove quotes from value if present
                value=$(echo "$value" | sed 's/^["'\'']//' | sed 's/["'\'']$//')
                ENV_VARS="$ENV_VARS##$key=$value"
                print_status "Added environment variable: $key"
            fi
        done < .env
        
        print_success "Environment variables loaded from .env file"
    else
        print_warning ".env file not found. Using default environment variables only."
        print_warning "Make sure to set required variables manually in Cloud Run console:"
        print_warning "  • GROQ_API_KEY"
        print_warning "  • SMTP_* variables for email notifications"
        print_warning "  • CV_PATH for your CV file"
        print_warning "  • JOB_LOCATIONS for target job locations"
    fi
    
    # Handle force clean deployment
    if [ "$FORCE_CLEAN" = true ]; then
        print_status "Force clean deployment enabled - clearing all caches..."
        
        # Clear local Docker caches if Docker is available
        if command -v docker &> /dev/null; then
            print_status "Clearing local Docker caches..."
            docker system prune -f 2>/dev/null || true
        fi
        
        # Add cache-busting environment variable
        ENV_VARS="$ENV_VARS##FORCE_CLEAN_DEPLOY=true"
    fi
    
    # Build with Cloud Build and deploy to Cloud Run (with cache busting)
    # Note: Adding BUILD_TAG as env var forces Cloud Run to recognize this as a new revision
    gcloud run deploy "$SERVICE_NAME" \
        --source . \
        --platform managed \
        --region "$REGION" \
        --allow-unauthenticated \
        --memory "$MEMORY" \
        --cpu "$CPU" \
        --max-instances "$MAX_INSTANCES" \
        --set-env-vars "$ENV_VARS" \
        --timeout 3600 \
        --project="$PROJECT_ID"
    
    print_success "Deployment completed"
}

get_service_url() {
    SERVICE_URL=$(gcloud run services describe "$SERVICE_NAME" \
        --platform managed \
        --region "$REGION" \
        --format 'value(status.url)' \
        --project="$PROJECT_ID")
    
    if [ -z "$SERVICE_URL" ]; then
        print_error "Failed to get service URL"
        exit 1
    fi
    
    print_success "Service URL: $SERVICE_URL"
}

setup_scheduler() {
    print_status "Setting up Google Cloud Scheduler..."
    
    # Check if scheduler job already exists
    if gcloud scheduler jobs describe "$SCHEDULER_JOB_NAME" --location="$REGION" --project="$PROJECT_ID" &> /dev/null; then
        print_warning "Scheduler job '$SCHEDULER_JOB_NAME' already exists. Updating..."
        
        gcloud scheduler jobs update http "$SCHEDULER_JOB_NAME" \
            --location="$REGION" \
            --schedule="$CRON_SCHEDULE" \
            --uri="$SERVICE_URL/run" \
            --http-method="POST" \
            --description="Automated job processing for Job Scorer" \
            --project="$PROJECT_ID"
    else
        print_status "Creating new scheduler job..."
        
        gcloud scheduler jobs create http "$SCHEDULER_JOB_NAME" \
            --location="$REGION" \
            --schedule="$CRON_SCHEDULE" \
            --uri="$SERVICE_URL/run" \
            --http-method="POST" \
            --description="Automated job processing for Job Scorer" \
            --project="$PROJECT_ID"
    fi
    
    print_success "Scheduler job configured with schedule: $CRON_SCHEDULE"
}

create_service_account() {
    print_status "Setting up service account for scheduler..."
    
    SA_NAME="job-scorer-scheduler"
    SA_EMAIL="${SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"
    
    # Create service account if it doesn't exist
    if ! gcloud iam service-accounts describe "$SA_EMAIL" --project="$PROJECT_ID" &> /dev/null; then
        gcloud iam service-accounts create "$SA_NAME" \
            --display-name="Job Scorer Scheduler" \
            --description="Service account for scheduled job processing" \
            --project="$PROJECT_ID"
        
        print_success "Service account created: $SA_EMAIL"
    else
        print_status "Service account already exists: $SA_EMAIL"
    fi
    
    # Grant Cloud Run Invoker role to service account
    gcloud run services add-iam-policy-binding "$SERVICE_NAME" \
        --member="serviceAccount:$SA_EMAIL" \
        --role="roles/run.invoker" \
        --region="$REGION" \
        --project="$PROJECT_ID"
    
    print_success "Service account permissions configured"
}

test_deployment() {
    print_status "Testing deployment..."
    
    # Test health endpoint
    if curl -f "$SERVICE_URL/health" > /dev/null 2>&1; then
        print_success "Health check passed"
    else
        print_error "Health check failed"
        exit 1
    fi
    
    print_status "Manual test: curl -X POST $SERVICE_URL/run"
}

show_summary() {
    print_success "🎉 Deployment completed successfully!"
    echo
    echo "📋 Deployment Summary:"
    echo "  • Service URL: $SERVICE_URL"
    echo "  • Build Tag: $BUILD_TAG"
    echo "  • Health Check: $SERVICE_URL/health"
    echo "  • Manual Trigger: $SERVICE_URL/run"
    echo "  • Stats Dashboard: $SERVICE_URL/stats"
    echo "  • Scheduler Job: $SCHEDULER_JOB_NAME"
    echo "  • Schedule: $CRON_SCHEDULE"
    echo
    echo "📊 Useful Commands:"
    echo "  • View logs: gcloud run services logs tail $SERVICE_NAME --region=$REGION"
    echo "  • Trigger manually: curl -X POST $SERVICE_URL/run"
    echo "  • View scheduler: gcloud scheduler jobs describe $SCHEDULER_JOB_NAME --location=$REGION"
    echo "  • Pause scheduler: gcloud scheduler jobs pause $SCHEDULER_JOB_NAME --location=$REGION"
    echo "  • Resume scheduler: gcloud scheduler jobs resume $SCHEDULER_JOB_NAME --location=$REGION"
    echo
    echo "🔧 Environment Variables:"
    if [ -f ".env" ]; then
        echo "  ✅ Loaded from .env file automatically"
    else
        echo "  ⚠️  No .env file found - set these manually in Cloud Run console:"
        echo "     • GROQ_API_KEY (required)"
        echo "     • SMTP_* variables for email notifications"
        echo "     • CV_PATH for your CV file"
        echo "     • JOB_LOCATIONS for target job locations"
    fi
    echo
    echo "💡 Pro Tips:"
    echo "  • Monitor execution in Cloud Scheduler console"
    echo "  • Check Cloud Run logs for job processing details"
    echo "  • Use the /stats endpoint to monitor performance"
    if [ "$FORCE_CLEAN" = true ]; then
        echo "  • Force clean deployment was used - all caches were cleared"
    else
        echo "  • Use --force-clean flag if deployment doesn't update properly"
    fi
}

main() {
    echo "🚀 Job Scorer Deployment Script"
    echo "================================="
    
    check_prerequisites
    enable_apis
    build_and_deploy
    get_service_url
    create_service_account
    setup_scheduler
    test_deployment
    show_summary
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --project)
            PROJECT_ID="$2"
            shift 2
            ;;
        --service)
            SERVICE_NAME="$2"
            shift 2
            ;;
        --region)
            REGION="$2"
            shift 2
            ;;
        --schedule)
            CRON_SCHEDULE="$2"
            shift 2
            ;;
        --skip-scheduler)
            SKIP_SCHEDULER=true
            shift
            ;;
        --force-clean)
            FORCE_CLEAN=true
            shift
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo
            echo "Options:"
            echo "  --project PROJECT_ID     Google Cloud Project ID"
            echo "  --service SERVICE_NAME   Cloud Run service name (default: job-scorer)"
            echo "  --region REGION          Cloud Run region (default: europe-west1)"
            echo "  --schedule SCHEDULE      Cron schedule (default: '0 */1 * * *')"
            echo "  --skip-scheduler         Skip scheduler setup"
            echo "  --force-clean            Force clean deployment (clears caches)"
            echo "  --help                   Show this help message"
            echo
            echo "Environment Variables:"
            echo "  GOOGLE_CLOUD_PROJECT     Google Cloud Project ID"
            echo "  CLOUD_RUN_SERVICE        Cloud Run service name"
            echo "  CLOUD_RUN_REGION         Cloud Run region"
            echo "  CRON_SCHEDULE            Cron schedule expression"
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

main