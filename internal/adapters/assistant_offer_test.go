package adapters

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssistantPublicOfferAndFailures(t *testing.T) {
	for _, mode := range []string{"valid", "unavailable", "redirect", "unknown", "invalid-link", "wrong-trial"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/offer" || r.Method != "GET" || r.Header.Get("Authorization") != "" || r.ContentLength > 0 {
					t.Fatal("offer leaked authentication or content")
				}
				switch mode {
				case "unavailable":
					w.WriteHeader(503)
					fmt.Fprint(w, `{"error":{"code":"service_unavailable","message":"Offline"}}`)
					return
				case "redirect":
					http.Redirect(w, r, "https://example.invalid", 307)
					return
				}
				body := map[string]any{"priceText": "360 € + VAT per lab / year", "trialRequests": 5, "trialUrl": "https://assistant.example/trial", "subscribeUrl": "https://example.org/assistant/"}
				if mode == "unknown" {
					body["extra"] = true
				}
				if mode == "invalid-link" {
					body["subscribeUrl"] = "javascript:alert(1)"
				}
				if mode == "wrong-trial" {
					body["trialRequests"] = 500
				}
				json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()
			offer, err := (AssistantClient{Endpoint: server.URL + "/v1"}).Offer(t.Context())
			if mode == "valid" {
				if err != nil || offer.TrialRequests != 5 || !strings.Contains(offer.PriceText, "360 €") {
					t.Fatal(offer, err)
				}
			} else if err == nil {
				t.Fatal("invalid offer accepted")
			}
		})
	}
}
