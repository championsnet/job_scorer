package multitenant

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/webhook"
)

func (s *Server) handleBillingCheckout(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if strings.TrimSpace(s.runtime.StripeSecretKey) == "" {
		writeJSONError(w, http.StatusNotImplemented, "stripe billing is not configured")
		return
	}
	if len(s.runtime.BillingPackages) == 0 {
		writeJSONError(w, http.StatusNotImplemented, "no billing packages configured")
		return
	}

	var payload struct {
		PackageID string `json:"package_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid checkout payload")
		return
	}
	payload.PackageID = strings.TrimSpace(payload.PackageID)
	if payload.PackageID == "" {
		payload.PackageID = s.runtime.BillingPackages[0].ID
	}

	var selected *CreditPackage
	for i := range s.runtime.BillingPackages {
		pkg := &s.runtime.BillingPackages[i]
		if pkg.ID == payload.PackageID {
			selected = pkg
			break
		}
	}
	if selected == nil {
		writeJSONError(w, http.StatusBadRequest, "unknown billing package")
		return
	}

	stripe.Key = s.runtime.StripeSecretKey
	successURL := s.runtime.StripeSuccessURL
	cancelURL := s.runtime.StripeCancelURL
	if successURL == "" {
		successURL = "https://example.com/billing/success?session_id={CHECKOUT_SESSION_ID}"
	}
	if cancelURL == "" {
		cancelURL = "https://example.com/billing/cancel"
	}

	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(selected.PriceID),
				Quantity: stripe.Int64(1),
			},
		},
		Metadata: map[string]string{
			"account_id": user.AccountID,
			"credits":    strconv.Itoa(selected.Credits),
			"package_id": selected.ID,
		},
	}

	created, err := session.New(params)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed creating checkout session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"session_id": created.ID,
		"url":        created.URL,
	})
}

func (s *Server) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.runtime.StripeSecretKey) == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "stripe not configured"})
		return
	}
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed reading webhook payload")
		return
	}

	var evt stripe.Event
	if strings.TrimSpace(s.runtime.StripeWebhookSecret) != "" {
		signature := r.Header.Get("Stripe-Signature")
		evt, err = webhook.ConstructEvent(payload, signature, s.runtime.StripeWebhookSecret)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid webhook signature")
			return
		}
	} else {
		if err := json.Unmarshal(payload, &evt); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid webhook payload")
			return
		}
	}

	switch evt.Type {
	case "checkout.session.completed":
		var cs stripe.CheckoutSession
		if err := json.Unmarshal(evt.Data.Raw, &cs); err != nil {
			writeJSONError(w, http.StatusBadRequest, "failed decoding checkout session")
			return
		}
		accountID := strings.TrimSpace(cs.Metadata["account_id"])
		if accountID == "" {
			writeJSONError(w, http.StatusBadRequest, "missing account_id metadata")
			return
		}
		credits, _ := strconv.Atoi(strings.TrimSpace(cs.Metadata["credits"]))
		if credits <= 0 {
			credits = creditsForPriceID(s.runtime.BillingPackages, cs.Metadata["package_id"], cs.LineItems)
		}
		if credits <= 0 {
			writeJSONError(w, http.StatusBadRequest, "missing credits metadata")
			return
		}
		if err := s.repo.RecordCreditsPurchase(r.Context(), accountID, evt.ID, payload, int(cs.AmountTotal), string(cs.Currency), credits); err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed recording payment: %v", err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "type": string(evt.Type)})
	}
}

func creditsForPriceID(pkgs []CreditPackage, packageID string, _ *stripe.LineItemList) int {
	packageID = strings.TrimSpace(packageID)
	for _, pkg := range pkgs {
		if packageID != "" && pkg.ID == packageID {
			return pkg.Credits
		}
	}
	return 0
}
