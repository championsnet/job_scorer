//go:build saas

package main

import (
	"context"

	"job-scorer/multitenant"
)

// runSaaS starts the multi-tenant service. Only compiled with -tags saas so the
// local/desktop app doesn't pull in Firebase/Firestore/Cloud Tasks/Stripe/gRPC.
func runSaaS(ctx context.Context) error {
	return multitenant.Run(ctx)
}
