package adapters

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (Local) KeyMaterial(repository string) []domain.KeyMaterialState {
	pairs := []struct {
		name        string
		privatePath string
		publicPath  string
	}{
		{"cache", "secret-key", filepath.Join("keys", "cache-public-key")},
		{"ssh", "admin-ssh", filepath.Join("keys", "admin-ssh.pub")},
		{"veyon", "veyon-private-key.pem", filepath.Join("keys", "veyon-public-key.pem")},
	}
	states := make([]domain.KeyMaterialState, 0, len(pairs))
	for _, pair := range pairs {
		state := domain.KeyMaterialState{Name: pair.name}
		privateContent, mode, privateErr := readRegularFileNoFollowLimit(filepath.Join(repository, pair.privatePath), 64*1024)
		switch {
		case privateErr == nil:
			state.PrivatePresent = len(privateContent) > 0
			state.PrivateMode = mode
			state.Safe = mode&0077 == 0
			if !state.PrivatePresent {
				state.Problem = "private key is empty"
			} else if !state.Safe {
				state.Problem = fmt.Sprintf("private key mode %04o permits group or other access", mode)
			}
		case !os.IsNotExist(privateErr):
			state.Problem = fmt.Sprintf("private key is unsafe or unreadable: %v", privateErr)
		}
		if publicContent, _, publicErr := readRegularFileNoFollowLimit(filepath.Join(repository, pair.publicPath), 64*1024); publicErr == nil {
			state.PublicPresent = len(publicContent) > 0
			if !state.PublicPresent && state.Problem == "" {
				state.Problem = "public key is empty"
			}
		} else if !os.IsNotExist(publicErr) && state.Problem == "" {
			state.Problem = fmt.Sprintf("public key is unsafe or unreadable: %v", publicErr)
		}
		states = append(states, state)
	}
	return states
}
