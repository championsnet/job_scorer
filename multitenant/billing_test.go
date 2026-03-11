package multitenant

import "testing"

func TestCreditsForPriceID(t *testing.T) {
	pkgs := []CreditPackage{
		{ID: "starter", Credits: 25},
		{ID: "pro", Credits: 120},
	}

	credits := creditsForPriceID(pkgs, "pro", nil)
	if credits != 120 {
		t.Fatalf("expected 120 credits, got %d", credits)
	}

	credits = creditsForPriceID(pkgs, "missing", nil)
	if credits != 0 {
		t.Fatalf("expected 0 credits for unknown package, got %d", credits)
	}
}
