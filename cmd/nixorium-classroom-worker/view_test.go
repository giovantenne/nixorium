package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeViewSource struct{}

func (fakeViewSource) Computers(context.Context) ([]app.ClassroomComputer, error) {
	return []app.ClassroomComputer{{Name: "pc01", State: app.ClassroomViewing, HasImage: true}}, nil
}

func (fakeViewSource) Thumbnail(name string) ([]byte, bool) {
	return []byte{0xff, 0xd8}, name == "pc01"
}

func viewRequest(server *viewServer, path, host, cookie string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Host = host
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: viewCookie, Value: cookie})
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

func TestClassroomViewNeedsTheOneTimeTokenAndTheLoopbackHost(t *testing.T) {
	server := newViewServer(fakeViewSource{})
	address, err := server.Open()
	if err != nil || !strings.HasPrefix(address, "http://127.0.0.1:") || !strings.Contains(address, "/open?token=") {
		t.Fatalf("address = %q, %v", address, err)
	}
	host := strings.TrimPrefix(strings.SplitN(address, "/open", 2)[0], "http://")
	path := "/open" + strings.SplitN(address, "/open", 2)[1]

	if response := viewRequest(server, "/api/computers", host, ""); response.Code != http.StatusForbidden {
		t.Fatalf("anonymous request = %d", response.Code)
	}
	if response := viewRequest(server, path, "evil.example:80", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("foreign host = %d", response.Code)
	}
	response := viewRequest(server, path, host, "")
	if response.Code != http.StatusSeeOther || len(response.Result().Cookies()) != 1 {
		t.Fatalf("enter = %d %v", response.Code, response.Result().Cookies())
	}
	cookie := response.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %+v", cookie)
	}
	if again := viewRequest(server, path, host, ""); again.Code != http.StatusForbidden {
		t.Fatalf("token reused = %d", again.Code)
	}
	page := viewRequest(server, "/", host, cookie.Value)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Classroom view") || !strings.Contains(page.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatalf("page = %d %q", page.Code, page.Header())
	}
	if list := viewRequest(server, "/api/computers", host, cookie.Value); list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"pc01"`) {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	if image := viewRequest(server, "/api/thumbnail?name=pc01", host, cookie.Value); image.Code != http.StatusOK || image.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("image = %d", image.Code)
	}
	for _, name := range []string{"../etc", "pc09"} {
		if image := viewRequest(server, "/api/thumbnail?name="+name, host, cookie.Value); image.Code == http.StatusOK {
			t.Fatalf("thumbnail %q served", name)
		}
	}
	// Expired tokens and sessions are refused.
	server.now = func() time.Time { return time.Now().Add(13 * time.Hour) }
	if expired := viewRequest(server, "/api/computers", host, cookie.Value); expired.Code != http.StatusForbidden {
		t.Fatalf("expired session = %d", expired.Code)
	}
}

func TestClassroomViewOpensOnlyWhenTurnedOn(t *testing.T) {
	worker := classroomWorker{view: newViewServer(fakeViewSource{}), viewOn: func() bool { return false }}
	response := worker.handle(context.Background(), domain.ClassroomRequest{Operation: domain.ClassroomViewOpenOperation})
	if response.State != "failed" || !strings.Contains(response.Message, "CLASSROOM-VIEW-OFF") || response.ViewURL != "" {
		t.Fatalf("response = %+v", response)
	}
	worker.viewOn = func() bool { return true }
	response = worker.handle(context.Background(), domain.ClassroomRequest{Operation: domain.ClassroomViewOpenOperation})
	if response.State != "completed" || !strings.HasPrefix(response.ViewURL, "http://127.0.0.1:") {
		t.Fatalf("response = %+v", response)
	}
}
