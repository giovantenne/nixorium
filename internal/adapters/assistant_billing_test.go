package adapters

import (
	"errors"
	"fmt"
	"github.com/giovantenne/nixorium/internal/domain"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssistantBillingPortalProtocol(t *testing.T) {
	for _, test := range []struct {
		name, response string
		status         int
		code           string
	}{
		{"session", `{"url":"https://billing.stripe.com/p/session?secret=test_synthetic"}`, 200, ""},
		{"trial", `{"error":{"code":"no_subscription","message":"Available after subscribing."}}`, 409, "no_subscription"},
		{"extra", `{"url":"https://billing.stripe.com/p/session?secret=test_synthetic","customer":"private"}`, 200, "protocol_error"},
		{"missing", `{}`, 200, "protocol_error"},
		{"script", `{"url":"javascript:alert(1)"}`, 200, "protocol_error"},
		{"foreign", `{"url":"https://other.example/session"}`, 200, "protocol_error"},
		{"credentials", `{"url":"https://private@billing.stripe.com/session"}`, 200, "protocol_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			token := strings.Repeat("t", 43)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/billing-portal" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer "+token {
					t.Error("wrong billing request")
				}
				body, _ := io.ReadAll(r.Body)
				if len(body) != 0 {
					t.Error("billing sent conversation content")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.response)
			}))
			defer server.Close()
			client := AssistantClient{Endpoint: server.URL + "/v1"}
			link, err := client.BillingPortal(t.Context(), token)
			if test.code == "" {
				if err != nil || link != "https://billing.stripe.com/p/session?secret=test_synthetic" {
					t.Fatal(link, err)
				}
			} else {
				var api *domain.AssistantError
				if !errors.As(err, &api) || api.Code != test.code || link != "" {
					t.Fatal(link, err)
				}
			}
		})
	}
}
