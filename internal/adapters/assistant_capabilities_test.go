package adapters

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAssistantHomeFilesCapabilityUsesPinnedPureMetadataOnce(t *testing.T) {
	for _, response := range []string{"true", "false", "1", "invalid", "failure"} {
		t.Run(response, func(t *testing.T) {
			workspace, _, _ := assistantWorkspaceFixture(t)
			calls := 0
			run := func(ctx context.Context, limit int, args ...string) (string, error) {
				calls++
				joined := strings.Join(args, " ")
				for _, expected := range []string{"--offline", "--no-write-lock-file", "--no-update-lock-file", "allow-import-from-derivation false", "accept-flake-config false", "rev=" + workspace.Conversation.Base, "f.inputs.nixorium.lib.homeFilesVersion or 0"} {
					if !strings.Contains(joined, expected) {
						t.Fatal("unsafe capability probe", args)
					}
				}
				if limit != 1024 {
					t.Fatal(limit)
				}
				if response == "failure" {
					return "", errors.New("synthetic unavailable metadata")
				}
				return response, nil
			}
			for i := 0; i < 2; i++ {
				if got := assistantHomeFilesSupported(t.Context(), workspace, run); got != (response == "true") {
					t.Fatal(response, got)
				}
			}
			if calls != 1 {
				t.Fatal("metadata evaluated repeatedly", calls)
			}
		})
	}
}
