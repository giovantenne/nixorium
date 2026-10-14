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
	for _, mode := range []string{"valid", "configured", "smallest", "largest", "unavailable", "redirect", "unknown", "invalid-link", "zero-trial", "negative-trial", "large-trial", "zero-days", "negative-days", "long-days", "string-trial"} {
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
					http.Redirect(w, r, "https://example.invalid", http.StatusTemporaryRedirect)
					return
				}
				body := map[string]any{"priceText": "360 € + VAT per lab / year", "trialRequests": 10, "trialUrl": "https://assistant.example/trial", "subscribeUrl": "https://example.org/assistant/"}
				if mode == "unknown" {
					body["extra"] = true
				}
				if mode == "invalid-link" {
					body["subscribeUrl"] = "javascript:alert(1)"
				}
				switch mode {
				case "configured":
					body["trialRequests"], body["trialDays"] = 30, 90
				case "smallest":
					body["trialRequests"], body["trialDays"] = 1, 1
				case "largest":
					body["trialRequests"], body["trialDays"] = 1000, 3650
				case "zero-trial":
					body["trialRequests"] = 0
				case "negative-trial":
					body["trialRequests"] = -1
				case "large-trial":
					body["trialRequests"] = 1001
				case "zero-days":
					// An explicit zero length is indistinguishable from an absent one.
					body["trialDays"] = 0
				case "negative-days":
					body["trialDays"] = -30
				case "long-days":
					body["trialDays"] = 3651
				case "string-trial":
					body["trialRequests"] = "30"
				}
				json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()
			offer, err := (AssistantClient{Endpoint: server.URL + "/v1"}).Offer(t.Context())
			want := map[string][2]int{"valid": {10, 0}, "zero-days": {10, 0}, "configured": {30, 90}, "smallest": {1, 1}, "largest": {1000, 3650}}
			if terms, ok := want[mode]; ok {
				if err != nil || offer.TrialRequests != terms[0] || offer.TrialDays != terms[1] || !strings.Contains(offer.PriceText, "360 €") {
					t.Fatal(offer, err)
				}
			} else if err == nil {
				t.Fatal("invalid offer accepted")
			}
		})
	}
}

func TestAssistantRequestsIdentifyTheClientVersion(t *testing.T) {
	for _, test := range []struct{ version, want string }{
		{"3.6.0", "3.6.0"}, {"3.6.0-rc.1+abc", "3.6.0-rc.1+abc"}, {"development", "development"},
		{"", ""}, {"bad version", ""}, {"3.6.0\r\nX-Injected: 1", ""}, {strings.Repeat("9", 65), ""},
	} {
		var got []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = append(got, r.Header.Get("X-Nixorium-Client-Version"))
			if r.Header.Get("X-Injected") != "" {
				t.Fatal("version header injected another header")
			}
			if r.URL.Path == "/v1/offer" {
				fmt.Fprint(w, `{"priceText":"","trialRequests":30,"trialDays":90,"trialUrl":"https://assistant.example/trial","subscribeUrl":"https://example.org/assistant/"}`)
				return
			}
			fmt.Fprint(w, `{"account":{"name":"Synthetic School","plan":"trial","status":"active","periodEnd":"2099-01-01","turnsRemaining":30,"trialRequests":30}}`)
		}))
		client := AssistantClient{Endpoint: server.URL + "/v1", Version: test.version}
		if _, err := client.Offer(t.Context()); err != nil {
			t.Fatal(err)
		}
		if _, err := client.Account(t.Context(), strings.Repeat("a", 43)); err != nil {
			t.Fatal(err)
		}
		server.Close()
		if len(got) != 2 || got[0] != test.want || got[1] != test.want {
			t.Fatalf("version %q sent %q, want %q", test.version, got, test.want)
		}
	}
}
