//go:build !saas

package main

import (
	"context"
	"fmt"
)

// runSaaS is a stub for local/desktop builds, which don't include the
// multi-tenant SaaS stack. Build the cloud image with -tags saas to enable it.
func runSaaS(_ context.Context) error {
	return fmt.Errorf("multi-tenant mode is not included in this build; rebuild with -tags saas")
}
