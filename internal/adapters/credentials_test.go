package adapters

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHashPasswordUsesOnlyStdin(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "mkpasswd")
	content := "#!/bin/sh\n" +
		"[ \"$1\" = '-m' ] && [ \"$2\" = 'sha-512' ] && [ \"$3\" = '--stdin' ] && [ \"$#\" = 3 ] || exit 20\n" +
		"IFS= read -r password\n" +
		"[ \"$password\" = 'test-secret' ] || exit 21\n" +
		"printf '%s\\n' '$6$salt$hash'\n"
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	hash, err := (Local{}).HashPassword(context.Background(), []byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if hash != "$6$salt$hash" {
		t.Fatalf("hash = %q", hash)
	}
}
