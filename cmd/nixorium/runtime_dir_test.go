package main

import "testing"

func TestSanitizeRuntimeDir(t *testing.T) {
	owners := map[string]int{"/run/user/1001": 1001, "/run/user/1000": 1000, "/run/user/9": 0}
	owner := func(p string) (int, bool) { u, ok := owners[p]; return u, ok }
	for _, test := range []struct {
		name, in, want string
		uid            int
	}{
		{"unset stays unset", "", "", 1000},
		{"own kept", "/run/user/1000", "/run/user/1000", 1000},
		{"foreign replaced", "/run/user/1001", "/run/user/1000", 1000},
		{"missing replaced", "/gone", "/run/user/1000", 1000},
		{"foreign without own dir unset", "/run/user/1001", "", 1234},
	} {
		env := map[string]string{}
		if test.in != "" {
			env["XDG_RUNTIME_DIR"] = test.in
		}
		sanitizeRuntimeDir(func(k string) string { return env[k] },
			func(k, v string) error { env[k] = v; return nil },
			func(k string) error { delete(env, k); return nil }, owner, test.uid)
		if env["XDG_RUNTIME_DIR"] != test.want {
			t.Errorf("%s: got %q want %q", test.name, env["XDG_RUNTIME_DIR"], test.want)
		}
	}
}
