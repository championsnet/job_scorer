package multitenant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	firebase "firebase.google.com/go/v4"
	firebaseauth "firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

type VerifiedIdentity struct {
	UID           string
	Email         string
	EmailVerified bool
}

type TokenVerifier interface {
	Verify(ctx context.Context, token string, r *http.Request) (*VerifiedIdentity, error)
}

type firebaseTokenVerifier struct {
	client *firebaseauth.Client
}

func (v *firebaseTokenVerifier) Verify(ctx context.Context, token string, _ *http.Request) (*VerifiedIdentity, error) {
	token = strings.TrimSpace(token)
	decoded, err := v.client.VerifyIDToken(ctx, token)
	if err != nil {
		return nil, err
	}
	email, _ := decoded.Claims["email"].(string)
	emailVerified, _ := decoded.Claims["email_verified"].(bool)
	return &VerifiedIdentity{
		UID:           decoded.UID,
		Email:         strings.ToLower(strings.TrimSpace(email)),
		EmailVerified: emailVerified,
	}, nil
}

type bypassTokenVerifier struct{}

func (v *bypassTokenVerifier) Verify(_ context.Context, token string, r *http.Request) (*VerifiedIdentity, error) {
	token = strings.TrimSpace(token)
	debugUser := strings.TrimSpace(r.Header.Get("X-Debug-User"))
	debugUID := strings.TrimSpace(r.Header.Get("X-Debug-UID"))

	if debugUser == "" && strings.HasPrefix(token, "dev:") {
		debugUser = strings.TrimPrefix(token, "dev:")
	}
	if debugUID == "" {
		debugUID = "debug-user"
	}
	if debugUser == "" {
		return nil, fmt.Errorf("AUTH_BYPASS is enabled but no debug identity was provided")
	}
	return &VerifiedIdentity{
		UID:           debugUID,
		Email:         strings.ToLower(debugUser),
		EmailVerified: true,
	}, nil
}

type AuthService struct {
	repo     *Repository
	verifier TokenVerifier
}

func NewAuthService(ctx context.Context, cfg *RuntimeConfig, repo *Repository) (*AuthService, error) {
	if cfg.AuthBypass {
		return &AuthService{
			repo:     repo,
			verifier: &bypassTokenVerifier{},
		}, nil
	}

	opts := make([]option.ClientOption, 0, 1)
	if cfg.FirebaseCredentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(cfg.FirebaseCredentialsFile))
	}

	fbCfg := &firebase.Config{
		ProjectID: cfg.FirebaseProjectID,
	}
	app, err := firebase.NewApp(ctx, fbCfg, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed creating firebase app: %w", err)
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed creating firebase auth client: %w", err)
	}
	return &AuthService{
		repo: repo,
		verifier: &firebaseTokenVerifier{
			client: client,
		},
	}, nil
}

type contextKey string

const userContextKey contextKey = "multiTenantUser"

func CurrentUser(r *http.Request) (*UserAccount, bool) {
	user, ok := r.Context().Value(userContextKey).(*UserAccount)
	return user, ok
}

func (a *AuthService) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
			return
		}

		identity, err := a.verifier.Verify(r.Context(), token, r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[AUTH] token verification failed: %v\n", err)
			writeJSONError(w, http.StatusUnauthorized, "invalid auth token")
			return
		}

		user, err := a.repo.EnsureUserAccount(r.Context(), identity.UID, identity.Email, identity.EmailVerified)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[AUTH] EnsureUserAccount failed for %s: %v\n", identity.Email, err)
			writeJSONError(w, http.StatusInternalServerError, "failed resolving user account")
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
