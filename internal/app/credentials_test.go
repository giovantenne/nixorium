package app

import (
	"context"
	"errors"
	"testing"
)

type queuedSecretReader struct {
	values [][]byte
}

func (r *queuedSecretReader) ReadSecret(string) ([]byte, error) {
	if len(r.values) == 0 {
		return nil, errors.New("no secret")
	}
	value := r.values[0]
	r.values = r.values[1:]
	return value, nil
}

type recordingHasher struct {
	input []byte
	calls int
}

func (h *recordingHasher) HashPassword(_ context.Context, password []byte) (string, error) {
	h.calls++
	h.input = append([]byte(nil), password...)
	return "$6$salt$hash", nil
}

func TestCollectPasswordHashConfirmsHashesAndWipesInputs(t *testing.T) {
	password := []byte("correct horse battery staple")
	confirmation := append([]byte(nil), password...)
	reader := &queuedSecretReader{values: [][]byte{password, confirmation}}
	hasher := &recordingHasher{}
	hash, err := CollectPasswordHash(context.Background(), reader, hasher)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "$6$salt$hash" || hasher.calls != 1 || string(hasher.input) != "correct horse battery staple" {
		t.Fatalf("hash = %q, hasher = %+v", hash, hasher)
	}
	for _, input := range [][]byte{password, confirmation} {
		for _, value := range input {
			if value != 0 {
				t.Fatal("plaintext input was not wiped")
			}
		}
	}
}

func TestCollectPasswordHashRejectsMismatchAndPublicDefault(t *testing.T) {
	for name, values := range map[string][][]byte{
		"mismatch": {[]byte("long-enough-one"), []byte("long-enough-two")},
		"default":  {[]byte("nixos"), []byte("nixos")},
	} {
		t.Run(name, func(t *testing.T) {
			hasher := &recordingHasher{}
			if _, err := CollectPasswordHash(context.Background(), &queuedSecretReader{values: values}, hasher); err == nil {
				t.Fatal("unsafe password was accepted")
			}
			if hasher.calls != 0 {
				t.Fatal("hasher ran for rejected input")
			}
		})
	}
}
