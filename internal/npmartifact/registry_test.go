package npmartifact

import (
	"context"
	"net/http"
	"testing"
)

func TestRegistryUsesTheSystemProxyAndNoRedirects(t *testing.T) {
	registry := NewRegistry(context.Background())
	transport, ok := registry.HTTP.Transport.(*http.Transport)
	if !ok || transport.Proxy == nil {
		t.Fatal("the npm registry client must read the system proxy (http_proxy, https_proxy, no_proxy)")
	}
	if registry.HTTP.CheckRedirect == nil {
		t.Fatal("the npm registry client must not follow redirects")
	}
}
