package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/systemfiles"
)

type fakeSystemFiles struct {
	meta       domain.LabMeta
	host       domain.HostMeta
	controller bool
	err        error
	calls      int
}

func (f *fakeSystemFiles) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return f.meta, nil
}
func (f *fakeSystemFiles) ReadSystemFile(_ context.Context, host domain.HostMeta, controller bool, path string) (systemfiles.Result, error) {
	f.calls++
	f.host, f.controller = host, controller
	return systemfiles.Result{Path: path, Kind: "file", Content: "set default=0\n"}, f.err
}

func TestSystemFileToolReadsOnlyDeclaredComputers(t *testing.T) {
	missing := errors.New("missing")
	source := &fakeSystemFiles{}
	source.meta.Controller.Name, source.meta.Controller.StaticIP = "pc99", "10.0.0.99"
	source.meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}}
	tool := AssistantSystemFiles{Source: source, Errors: map[error]string{missing: "not_found"}}

	if got := tool.Execute(context.Background(), "/lab", "pc01", "/boot/grub/grub.cfg"); !strings.Contains(got, `"ok":true`) || source.host.IP != "10.0.0.1" || source.controller {
		t.Fatalf("client read: %s %+v", got, source.host)
	}
	if got := tool.Execute(context.Background(), "/lab", "pc99", "/etc/hosts"); !strings.Contains(got, `"ok":true`) || !source.controller {
		t.Fatalf("controller read: %s", got)
	}
	calls := source.calls
	for _, test := range []struct{ host, path, code string }{
		{"pc42", "/etc/hosts", "unknown_host"},
		{"pc01", "/etc/shadow", "refused_path"},
		{"pc01", "/home/student/x", "refused_path"},
		{"pc01", "/etc/../root", "invalid_arguments"},
		{"pc01;id", "/etc/hosts", "invalid_arguments"},
	} {
		if got := tool.Execute(context.Background(), "/lab", test.host, test.path); !strings.Contains(got, test.code) {
			t.Errorf("%s %s: %s", test.host, test.path, got)
		}
	}
	if source.calls != calls {
		t.Fatal("a refused request reached a computer")
	}
	source.err = missing
	if got := tool.Execute(context.Background(), "/lab", "pc01", "/etc/hosts"); !strings.Contains(got, "not_found") {
		t.Fatal(got)
	}
}
