package app

import (
	"context"
	"errors"
	"testing"
)

type queuedSecretReader struct {
	values  [][]byte
	prompts []string
}

func (r *queuedSecretReader) ReadSecret(prompt string) ([]byte, error) {
	r.prompts = append(r.prompts, prompt)
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

func TestCollectNamedPasswordHashConfirmsHashesAndWipesInputs(t *testing.T) {
	password := []byte("correct horse battery staple")
	confirmation := append([]byte(nil), password...)
	reader := &queuedSecretReader{values: [][]byte{password, confirmation}}
	hasher := &recordingHasher{}
	hash, err := CollectNamedPasswordHash(context.Background(), reader, hasher, "Password")
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

func TestCollectNamedPasswordHashRejectsMismatchAndPublicDefault(t *testing.T) {
	for name, values := range map[string][][]byte{
		"mismatch": {[]byte("long-enough-one"), []byte("long-enough-two")},
		"default":  {[]byte("nixos")},
	} {
		t.Run(name, func(t *testing.T) {
			inputs := append([][]byte(nil), values...)
			reader := &queuedSecretReader{values: inputs}
			hasher := &recordingHasher{}
			if _, err := CollectNamedPasswordHash(context.Background(), reader, hasher, "Password"); err == nil || !IsPasswordInputError(err) {
				t.Fatal("unsafe password was accepted")
			}
			if hasher.calls != 0 {
				t.Fatal("hasher ran for rejected input")
			}
			for _, input := range inputs {
				for _, value := range input {
					if value != 0 {
						t.Fatal("rejected plaintext input was not wiped")
					}
				}
			}
			if name == "default" && len(reader.prompts) != 1 {
				t.Fatalf("public default unnecessarily requested confirmation: %v", reader.prompts)
			}
		})
	}
}

func TestCollectNamedPasswordHashRejectsShortInputBeforeConfirmation(t *testing.T) {
	password := []byte("short")
	reader := &queuedSecretReader{values: [][]byte{password}}
	hasher := &recordingHasher{}
	_, err := CollectNamedPasswordHash(context.Background(), reader, hasher, "Password")
	if err == nil || !IsPasswordInputError(err) {
		t.Fatalf("error = %v", err)
	}
	if len(reader.prompts) != 1 || hasher.calls != 0 {
		t.Fatalf("prompts = %v, hasher calls = %d", reader.prompts, hasher.calls)
	}
	for _, value := range password {
		if value != 0 {
			t.Fatal("short plaintext input was not wiped")
		}
	}
}

func TestCollectNamedPasswordHashDoesNotClassifyBackendFailureAsInputError(t *testing.T) {
	reader := &queuedSecretReader{values: [][]byte{[]byte("long-enough")}}
	_, err := CollectNamedPasswordHash(context.Background(), reader, &recordingHasher{}, "Password")
	if err == nil || IsPasswordInputError(err) {
		t.Fatalf("terminal failure classification = %v", err)
	}
}
