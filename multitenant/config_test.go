package multitenant

import (
	"os"
	"testing"
)

func TestParseBillingPackagesFromJSON(t *testing.T) {
	original := os.Getenv("BILLING_PACKAGES_JSON")
	defer func() {
		if original == "" {
			_ = os.Unsetenv("BILLING_PACKAGES_JSON")
		} else {
			_ = os.Setenv("BILLING_PACKAGES_JSON", original)
		}
	}()

	_ = os.Setenv("BILLING_PACKAGES_JSON", `[{"id":"starter","name":"Starter","description":"desc","credits":25,"price_id":"price_123"}]`)

	pkgs, err := parseBillingPackages()
	if err != nil {
		t.Fatalf("parseBillingPackages returned error: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].ID != "starter" || pkgs[0].Credits != 25 {
		t.Fatalf("unexpected package payload: %+v", pkgs[0])
	}
}

func TestParseBillingPackagesInvalidJSON(t *testing.T) {
	original := os.Getenv("BILLING_PACKAGES_JSON")
	defer func() {
		if original == "" {
			_ = os.Unsetenv("BILLING_PACKAGES_JSON")
		} else {
			_ = os.Setenv("BILLING_PACKAGES_JSON", original)
		}
	}()
	_ = os.Setenv("BILLING_PACKAGES_JSON", `invalid`)

	if _, err := parseBillingPackages(); err == nil {
		t.Fatalf("expected parse error for invalid BILLING_PACKAGES_JSON")
	}
}
