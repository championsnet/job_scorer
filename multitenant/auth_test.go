package multitenant

import "testing"

func TestBearerToken(t *testing.T) {
	token := bearerToken("Bearer abc123")
	if token != "abc123" {
		t.Fatalf("expected token abc123, got %q", token)
	}
}

func TestBearerToken_InvalidHeader(t *testing.T) {
	if token := bearerToken("Basic xyz"); token != "" {
		t.Fatalf("expected empty token for non-bearer header, got %q", token)
	}
	if token := bearerToken(""); token != "" {
		t.Fatalf("expected empty token for empty header, got %q", token)
	}
}
