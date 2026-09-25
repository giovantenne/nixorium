package adapters

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

type bootstrapSSHServer struct {
	listener   net.Listener
	signer     ssh.Signer
	mutex      sync.Mutex
	password   string
	locked     bool
	rejectRoot bool
	authorized map[string]bool
	closed     chan struct{}
}

func newBootstrapSSHServer(t *testing.T) *bootstrapSSHServer {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &bootstrapSSHServer{listener: listener, signer: signer, password: "temporary-secret", authorized: map[string]bool{}, closed: make(chan struct{})}
	go server.serve()
	t.Cleanup(func() {
		_ = listener.Close()
		<-server.closed
	})
	return server
}

func (server *bootstrapSSHServer) port() int {
	return server.listener.Addr().(*net.TCPAddr).Port
}

func (server *bootstrapSSHServer) fingerprint() string {
	return ssh.FingerprintSHA256(server.signer.PublicKey())
}

func (server *bootstrapSSHServer) serve() {
	defer close(server.closed)
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			return
		}
		go server.serveConnection(connection)
	}
}

func (server *bootstrapSSHServer) serveConnection(connection net.Conn) {
	configuration := &ssh.ServerConfig{
		PasswordCallback: func(metadata ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			server.mutex.Lock()
			defer server.mutex.Unlock()
			if metadata.User() == "nixos" && !server.locked && string(password) == server.password {
				return nil, nil
			}
			return nil, errors.New("password denied")
		},
		PublicKeyCallback: func(metadata ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			server.mutex.Lock()
			defer server.mutex.Unlock()
			if metadata.User() == "root" && !server.rejectRoot && server.authorized[string(key.Marshal())] {
				return nil, nil
			}
			return nil, errors.New("public key denied")
		},
	}
	configuration.AddHostKey(server.signer)
	serverConnection, channels, requests, err := ssh.NewServerConn(connection, configuration)
	if err != nil {
		_ = connection.Close()
		return
	}
	defer serverConnection.Close()
	go ssh.DiscardRequests(requests)
	for channelRequest := range channels {
		if channelRequest.ChannelType() != "session" {
			_ = channelRequest.Reject(ssh.UnknownChannelType, "session required")
			continue
		}
		channel, requests, err := channelRequest.Accept()
		if err != nil {
			continue
		}
		go server.serveSession(channel, requests)
	}
}

func (server *bootstrapSSHServer) serveSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	for request := range requests {
		if request.Type != "exec" || len(request.Payload) < 4 {
			_ = request.Reply(false, nil)
			continue
		}
		length := binary.BigEndian.Uint32(request.Payload[:4])
		if int(length) != len(request.Payload)-4 {
			_ = request.Reply(false, nil)
			continue
		}
		command := string(request.Payload[4:])
		_ = request.Reply(true, nil)
		input, _ := io.ReadAll(io.LimitReader(channel, 16*1024))
		output, status := server.execute(command, input)
		_, _ = channel.Write([]byte(output))
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(status)}))
		return
	}
}

func (server *bootstrapSSHServer) execute(command string, input []byte) (string, int) {
	switch command {
	case "/run/current-system/sw/bin/cat /etc/os-release":
		return "NAME=NixOS\nVARIANT_ID=installer\nVERSION_ID=\"26.05\"\nBUILD_ID=26.05.test\n", 0
	case "/run/current-system/sw/bin/uname -m":
		return "x86_64\n", 0
	case "/run/current-system/sw/bin/test -d /sys/firmware/efi",
		"/run/wrappers/bin/sudo -n /run/current-system/sw/bin/true",
		"/run/current-system/sw/bin/true",
		"/run/wrappers/bin/sudo -n /run/current-system/sw/bin/install -d -m 0700 -o root -g root /root/.ssh",
		"/run/wrappers/bin/sudo -n /run/current-system/sw/bin/chown root:root /root/.ssh/authorized_keys",
		"/run/wrappers/bin/sudo -n /run/current-system/sw/bin/chmod 0600 /root/.ssh/authorized_keys":
		return "", 0
	case "/run/current-system/sw/bin/cat /proc/sys/kernel/random/boot_id":
		return "01234567-89ab-cdef-0123-456789abcdef\n", 0
	case "/run/current-system/sw/bin/ip -j -4 address show scope global":
		return `[{"ifname":"enp0s2","link_type":"ether","addr_info":[{"family":"inet","local":"192.0.2.20"}]}]`, 0
	case "/run/wrappers/bin/sudo -n /run/current-system/sw/bin/tee -a /root/.ssh/authorized_keys":
		line := strings.TrimSpace(string(input))
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimPrefix(line, "restrict ")))
		if err != nil {
			return "invalid key", 1
		}
		server.mutex.Lock()
		server.authorized[string(key.Marshal())] = true
		server.mutex.Unlock()
		return line + "\n", 0
	case "/run/wrappers/bin/sudo -n /run/current-system/sw/bin/passwd -l nixos":
		server.mutex.Lock()
		server.locked = true
		server.mutex.Unlock()
		return "", 0
	default:
		if strings.Contains(command, ".authorized_keys.XXXXXX") {
			line := strings.TrimSpace(string(input))
			key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimPrefix(line, "restrict ")))
			if err == nil {
				server.mutex.Lock()
				delete(server.authorized, string(key.Marshal()))
				server.mutex.Unlock()
			}
			return "", 0
		}
		return "unsupported command", 127
	}
}

func TestLiveBootstrapPinsHostTransitionsCredentialAndKeepsOnlyRuntimeKey(t *testing.T) {
	server := newBootstrapSSHServer(t)
	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeRoot, 0700); err != nil {
		t.Fatal(err)
	}
	bootstrap := NewLiveBootstrap()
	bootstrap.runtimeRoot = runtimeRoot
	bootstrap.port = server.port()
	input := []byte("temporary-secret")
	password, err := NewLivePassword(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(input, make([]byte, len(input))) {
		t.Fatal("password constructor did not clear caller-owned bytes")
	}
	session, err := bootstrap.Establish(context.Background(), "0123456789abcdef0123456789abcdef", "127.0.0.1", server.fingerprint(), password)
	if err != nil {
		t.Fatal(err)
	}
	if len(password.value) != 0 || !server.locked || session.Facts.BootID != "01234567-89ab-cdef-0123-456789abcdef" || len(session.Facts.Interfaces) != 1 {
		t.Fatalf("session=%+v passwordBytes=%d locked=%t", session, len(password.value), server.locked)
	}
	for _, path := range []string{session.PrivateKeyPath, session.KnownHostsPath} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("runtime file %s info=%v error=%v", path, info, err)
		}
	}
	data, err := os.ReadFile(session.KnownHostsPath)
	if err != nil {
		t.Fatal(err)
	}
	_, hosts, key, _, _, err := ssh.ParseKnownHosts(data)
	if err != nil || len(hosts) != 1 || ssh.FingerprintSHA256(key) != server.fingerprint() {
		t.Fatalf("known_hosts does not pin the verified server: hosts=%v error=%v", hosts, err)
	}
}

func TestLiveBootstrapRejectsWrongFingerprintBeforeAuthorization(t *testing.T) {
	server := newBootstrapSSHServer(t)
	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeRoot, 0700); err != nil {
		t.Fatal(err)
	}
	bootstrap := NewLiveBootstrap()
	bootstrap.runtimeRoot = runtimeRoot
	bootstrap.port = server.port()
	secret, _ := NewLivePassword([]byte("temporary-secret"))
	_, err := bootstrap.Establish(context.Background(), "0123456789abcdef0123456789abcdef", "127.0.0.1", "SHA256:"+strings.Repeat("A", 43), secret)
	if err == nil || !strings.Contains(err.Error(), "fingerprint differs") {
		t.Fatalf("wrong fingerprint error=%v", err)
	}
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if len(server.authorized) != 0 || server.locked {
		t.Fatal("wrong host key modified the live credential state")
	}
}

func TestLiveBootstrapRejectsWrongPasswordWithoutAuthorization(t *testing.T) {
	server := newBootstrapSSHServer(t)
	bootstrap := NewLiveBootstrap()
	bootstrap.runtimeRoot = t.TempDir()
	bootstrap.port = server.port()
	secret, err := NewLivePassword([]byte("wrong-password"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = bootstrap.Establish(context.Background(), "0123456789abcdef0123456789abcdef", "127.0.0.1", server.fingerprint(), secret)
	if err == nil || !strings.Contains(err.Error(), "authenticate") {
		t.Fatalf("wrong password error=%v", err)
	}
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if len(server.authorized) != 0 || server.locked {
		t.Fatal("wrong password modified live authorization state")
	}
}

func TestLivePasswordWipesRejectedInput(t *testing.T) {
	input := []byte{'b', 'a', 'd', 0, 's', 'e', 'c', 'r', 'e', 't'}
	if _, err := NewLivePassword(input); err == nil {
		t.Fatal("NUL-containing live password was accepted")
	}
	for _, value := range input {
		if value != 0 {
			t.Fatal("rejected password input was not wiped")
		}
	}
}

func TestLiveBootstrapRevokesKeyWhenRootVerificationFails(t *testing.T) {
	server := newBootstrapSSHServer(t)
	server.rejectRoot = true
	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeRoot, 0700); err != nil {
		t.Fatal(err)
	}
	bootstrap := NewLiveBootstrap()
	bootstrap.runtimeRoot = runtimeRoot
	bootstrap.port = server.port()
	secret, _ := NewLivePassword([]byte("temporary-secret"))
	operationID := "0123456789abcdef0123456789abcdef"
	if _, err := bootstrap.Establish(context.Background(), operationID, "127.0.0.1", server.fingerprint(), secret); err == nil {
		t.Fatal("failed root verification was accepted")
	}
	server.mutex.Lock()
	authorized := len(server.authorized)
	server.mutex.Unlock()
	if authorized != 0 {
		t.Fatal("ephemeral key was not revoked after bootstrap failure")
	}
	if _, err := os.Lstat(filepath.Join(runtimeRoot, operationID)); !os.IsNotExist(err) {
		t.Fatalf("failed bootstrap left runtime credentials: %v", err)
	}
}

func TestInitialLiveProbeRejectsAmbiguousToolOutput(t *testing.T) {
	if _, err := parseOSRelease([]byte("VARIANT_ID=installer\nVARIANT_ID=other\n")); err == nil {
		t.Fatal("duplicate os-release key accepted")
	}
	for _, value := range []string{
		`[{"ifname":"enp0s2","ifname":"enp0s3","addr_info":[]}]`,
		`[{"ifname":"enp0s2","addr_info":[]}] {}`,
	} {
		if _, err := parseLiveInterfaces([]byte(value)); err == nil {
			t.Fatalf("ambiguous network inventory accepted: %s", value)
		}
	}
}
