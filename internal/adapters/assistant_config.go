package adapters

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// LoadAssistantConfig fails closed and never follows a symbolic link or blocks
// on a special file. Only an installed controller configuration enables it.
func LoadAssistantConfig(path string) (domain.AssistantConfig, error) {
	disabled := domain.AssistantConfig{}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return disabled, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return disabled, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return disabled, errors.New("invalid Assistant configuration file")
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return disabled, err
	}
	if len(data) > 4096 {
		return disabled, errors.New("Assistant configuration is too large")
	}
	var wire struct {
		SchemaVersion int    `json:"schemaVersion"`
		Enabled       bool   `json:"enabled"`
		Endpoint      string `json:"endpoint"`
		SubscribeURL  string `json:"subscribeUrl"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return disabled, errors.New("invalid Assistant configuration JSON")
	}
	if decoder.Decode(&struct{}{}) != io.EOF || wire.SchemaVersion != 1 ||
		!assistantConfigURL(wire.Endpoint, true) || !assistantConfigURL(wire.SubscribeURL, false) {
		return disabled, errors.New("invalid Assistant configuration")
	}
	return domain.AssistantConfig{Enabled: wire.Enabled, Endpoint: wire.Endpoint, SubscribeURL: wire.SubscribeURL}, nil
}

func assistantConfigURL(value string, localEndpoint bool) bool {
	if strings.Contains(value, "#") {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery ||
		u.Fragment != "" || u.Opaque != "" {
		return false
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return false
		}
	}
	if u.Scheme == "https" {
		return true
	}
	return localEndpoint && u.Scheme == "http" && u.Port() != "" &&
		(u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost")
}

type AssistantStateStore struct{ Directory string }

func (store AssistantStateStore) root() (*os.File, error) {
	fd, err := unix.Open(store.Directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	if err := telemetrySafeFile(fd, unix.S_IFDIR); err != nil {
		unix.Close(fd)
		return nil, errors.New("unsafe Assistant state directory")
	}
	return os.NewFile(uintptr(fd), store.Directory), nil
}

func assistantReadState(root *os.File, name string) ([]byte, error) {
	return assistantReadStateLimit(root, name, 4096)
}
func assistantReadStateLimit(root *os.File, name string, limit int) ([]byte, error) {
	fd, err := unix.Openat(int(root.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	if telemetrySafeFile(fd, unix.S_IFREG) != nil {
		return nil, errors.New("unsafe Assistant state file")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if len(data) > limit {
		return nil, errors.New("Assistant state file is too large")
	}
	return data, err
}

func assistantWriteState(root *os.File, name string, data []byte) error {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	temporary := ".assistant-" + hex.EncodeToString(random[:])
	fd, err := unix.Openat(int(root.Fd()), temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), temporary)
	defer file.Close()
	defer unix.Unlinkat(int(root.Fd()), temporary, 0)
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(int(root.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err == nil {
		if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
			return errors.New("unsafe Assistant state target")
		}
	} else if !errors.Is(err, unix.ENOENT) {
		return err
	}
	if err := unix.Renameat(int(root.Fd()), temporary, int(root.Fd()), name); err != nil {
		return err
	}
	return root.Sync()
}

func (store AssistantStateStore) ReadToken() (string, error) {
	root, err := store.root()
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return "", nil
		}
		return "", err
	}
	defer root.Close()
	data, err := assistantReadState(root, "token")
	if errors.Is(err, unix.ENOENT) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if !assistantTokenPattern.MatchString(token) {
		return "", errors.New("invalid saved Assistant token")
	}
	return token, nil
}

func (store AssistantStateStore) WriteToken(token string) error {
	if !assistantTokenPattern.MatchString(token) {
		return errors.New("invalid Assistant token")
	}
	root, err := store.root()
	if err != nil {
		return err
	}
	defer root.Close()
	// Every activation requires fresh consent, including account changes.
	if err := unix.Unlinkat(int(root.Fd()), "consent.json", 0); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return assistantWriteState(root, "token", []byte(token+"\n"))
}

func (store AssistantStateStore) DeleteToken() error {
	root, err := store.root()
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	for _, name := range []string{"token", "consent.json"} {
		if err := unix.Unlinkat(int(root.Fd()), name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			return err
		}
	}
	return root.Sync()
}

func (store AssistantStateStore) ReadConsent() (bool, error) {
	root, err := store.root()
	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer root.Close()
	data, err := assistantReadState(root, "consent.json")
	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var consent domain.AssistantConsent
	if assistantDecodeStrict(data, &consent) != nil {
		return false, errors.New("invalid Assistant consent")
	}
	_, err = time.Parse(time.RFC3339, consent.AcceptedAt)
	return err == nil && consent.SchemaVersion == 1 && consent.TextVersion == 1, nil
}

func (store AssistantStateStore) WriteConsent() error {
	return store.WriteConsentSharing(false)
}

func (store AssistantStateStore) WriteConsentSharing(share bool) error {
	root, err := store.root()
	if err != nil {
		return err
	}
	defer root.Close()
	data, err := json.Marshal(domain.AssistantConsent{SchemaVersion: 1, AcceptedAt: time.Now().UTC().Format(time.RFC3339), TextVersion: 1, ShareConversations: share})
	if err != nil {
		return err
	}
	return assistantWriteState(root, "consent.json", append(data, '\n'))
}

func (store AssistantStateStore) ReadSharing() (bool, error) {
	consent, err := store.sharingConsent()
	return consent.ShareConversations, err
}
func (store AssistantStateStore) sharingConsent() (domain.AssistantConsent, error) {
	var consent domain.AssistantConsent
	accepted, err := store.ReadConsent()
	if err != nil || !accepted {
		return consent, err
	}
	root, err := store.root()
	if err != nil {
		return consent, err
	}
	defer root.Close()
	data, err := assistantReadState(root, "consent.json")
	if err != nil {
		return consent, err
	}
	if assistantDecodeStrict(data, &consent) != nil {
		return domain.AssistantConsent{}, errors.New("invalid Assistant consent")
	}
	if _, err := time.Parse(time.RFC3339, consent.AcceptedAt); err != nil || consent.SchemaVersion != 1 || consent.TextVersion != 1 {
		return domain.AssistantConsent{}, nil
	}
	return consent, nil
}
func (store AssistantStateStore) WriteSharing(share bool) error {
	consent, err := store.sharingConsent()
	if err != nil {
		return err
	}
	if consent.AcceptedAt == "" {
		return errors.New("Accept Assistant consent before changing conversation sharing.")
	}
	consent.ShareConversations = share
	root, err := store.root()
	if err != nil {
		return err
	}
	defer root.Close()
	data, err := json.Marshal(consent)
	if err != nil {
		return err
	}
	return assistantWriteState(root, "consent.json", append(data, '\n'))
}
