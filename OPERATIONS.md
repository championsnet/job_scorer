# Operations and Hardening

This document captures the minimum production checks for the multi-tenant deployment.

## Health and Readiness

- `GET /health` verifies Firestore connectivity; returns `503` on failure.
- Multi-tenant services (`APP_MODE=web|worker`) should not silently fallback to local disk for GCS errors.

## Structured Logging Checklist

When reviewing logs for incidents, always include:
- `account_id`
- `run_id`
- `trigger_type` (`manual` or `scheduled`)
- `request_id`
- `status` (`queued`, `running`, `success`, `failed`)

## Alerting Recommendations

Configure alerts in Cloud Monitoring for:
- Cloud Run `web` service 5xx ratio and latency spikes.
- Cloud Run `worker` service 5xx ratio and timeout errors.
- Cloud Scheduler failures for dispatch job.
- Cloud Tasks backlog growth and task retry spikes.
- Stripe webhook failures (`/api/v1/billing/webhook` non-2xx).
- Firestore read/write error rates and high latency.

## Manual Smoke Test Checklist

After each deployment:
1. `curl <web-url>/health` returns `200`.
2. `curl <worker-url>/health` returns `200`.
3. Sign in from frontend and load dashboard.
4. Save settings and upload CV from settings page.
5. Trigger a manual run and verify status transitions (`queued -> running -> success/failed`).
6. Verify billing summary endpoint returns balance/packages.
7. Confirm scheduler dispatch endpoint is callable only by authorized scheduler identity + token.

## Failure Recovery Notes

- Queueing failure after run creation triggers run failure + credit refund.
- Worker run failures transition run to `failed` and refund reserved credits.
