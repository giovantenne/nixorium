package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeWorkerState struct {
	sessions map[string]domain.RemoteInstallSession
	saveErr  error
}

func (state *fakeWorkerState) Load(id string) (domain.RemoteInstallSession, error) {
	session, ok := state.sessions[id]
	if !ok {
		return domain.RemoteInstallSession{}, errors.New("missing")
	}
	return session, nil
}

func (state *fakeWorkerState) Save(session domain.RemoteInstallSession) error {
	if state.saveErr != nil {
		return state.saveErr
	}
	state.sessions[session.OperationID] = session
	return nil
}

type fakeWorkerBootstrap struct {
	session adapters.VerifiedLiveSession
	err     error
	closed  bool
}

func (bootstrap *fakeWorkerBootstrap) Establish(_ context.Context, operationID, address, fingerprint string, secret *adapters.LivePassword) (adapters.VerifiedLiveSession, error) {
	bootstrap.session.OperationID = operationID
	bootstrap.session.Address = address
	bootstrap.session.HostFingerprint = fingerprint
	return bootstrap.session, bootstrap.err
}

func (bootstrap *fakeWorkerBootstrap) CloseSession(context.Context, adapters.VerifiedLiveSession) error {
	bootstrap.closed = true
	return nil
}

func (bootstrap *fakeWorkerBootstrap) DiscardLocalSession(adapters.VerifiedLiveSession) error {
	bootstrap.closed = true
	return nil
}

type fakeWorkerReservation struct{ released bool }

func (reservation *fakeWorkerReservation) ReleaseResolved() error {
	reservation.released = true
	return nil
}

type fakeWorkerReservations struct {
	reservation *fakeWorkerReservation
	err         error
}

type fakeWorkerPreparer struct {
	preparation domain.RemoteInstallPreparation
	err         error
	discarded   bool
}

func (preparer *fakeWorkerPreparer) Prepare(_ context.Context, operationID, host string, live adapters.VerifiedLiveSession) (domain.RemoteInstallPreparation, error) {
	preparer.preparation.OperationID = operationID
	preparer.preparation.Host.Name = host
	preparer.preparation.Host.LiveIP = live.Address
	return preparer.preparation, preparer.err
}

func (preparer *fakeWorkerPreparer) Discard(string) error {
	preparer.discarded = true
	return nil
}

func (source *fakeWorkerReservations) ReserveRemoteSession(string) (domain.RemoteInstallReservation, error) {
	if source.err != nil {
		return nil, source.err
	}
	source.reservation = &fakeWorkerReservation{}
	return source.reservation, nil
}

func TestRemoteWorkerBootstrapPersistsNonSecretStateAndCancelCleansUp(t *testing.T) {
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{}}
	bootstrap := &fakeWorkerBootstrap{session: validWorkerLiveSession()}
	reservations := &fakeWorkerReservations{}
	worker := &remoteWorker{state: state, bootstrap: bootstrap, reservations: reservations}
	secret, err := adapters.NewLivePassword([]byte("temporary-secret"))
	if err != nil {
		t.Fatal(err)
	}
	request := domain.RemoteInstallRequest{
		SchemaVersion: domain.RemoteInstallSchemaVersion, RequestID: "0123456789abcdef0123456789abcdef",
		Operation: domain.RemoteInstallBootstrapOperation, Host: "pc01", Address: "192.0.2.20",
		Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	response := worker.handle(context.Background(), request, secret)
	if response.State != "bootstrapped" || response.OperationID == "" || response.Session == nil || reservations.reservation.released {
		t.Fatalf("bootstrap response=%+v reservation=%+v", response, reservations.reservation)
	}
	stored := state.sessions[response.OperationID]
	if stored.Bootstrap == nil || stored.Bootstrap.Host != "pc01" || stored.Bootstrap.Address != "192.0.2.20" || stored.TokenConsumed {
		t.Fatalf("stored session=%+v", stored)
	}
	persisted, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"temporary-secret", "/run/nixorium/remote-install/test/id_ed25519", "PRIVATE KEY"} {
		if strings.Contains(string(persisted), forbidden) {
			t.Fatalf("persistent state leaked %q: %s", forbidden, persisted)
		}
	}
	cancel := worker.handle(context.Background(), domain.RemoteInstallRequest{Operation: domain.RemoteInstallCancelOperation, OperationID: response.OperationID}, nil)
	if cancel.State != "cancelled" || !bootstrap.closed || !reservations.reservation.released || worker.operationID != "" {
		t.Fatalf("cancel=%+v worker=%+v", cancel, worker)
	}
}

func TestRemoteWorkerConfirmedBootstrapFailureReleasesReservation(t *testing.T) {
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{}}
	bootstrap := &fakeWorkerBootstrap{err: errors.New("password denied")}
	reservations := &fakeWorkerReservations{}
	worker := &remoteWorker{state: state, bootstrap: bootstrap, reservations: reservations}
	secret, _ := adapters.NewLivePassword([]byte("wrong"))
	response := worker.handle(context.Background(), domain.RemoteInstallRequest{
		Operation: domain.RemoteInstallBootstrapOperation, Host: "pc01", Address: "192.0.2.20",
		Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}, secret)
	if response.State != "blocked" || reservations.reservation == nil || !reservations.reservation.released || worker.operationID != "" {
		t.Fatalf("failure response=%+v reservation=%+v", response, reservations.reservation)
	}
}

func TestRemoteWorkerPreparationPersistsNonSecretVerifiedFacts(t *testing.T) {
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{}}
	bootstrap := &fakeWorkerBootstrap{session: validWorkerLiveSession()}
	reservations := &fakeWorkerReservations{}
	preparer := &fakeWorkerPreparer{preparation: validWorkerPreparation()}
	worker := &remoteWorker{state: state, bootstrap: bootstrap, preparer: preparer, reservations: reservations}
	secret, _ := adapters.NewLivePassword([]byte("temporary-secret"))
	bootstrapResponse := worker.handle(context.Background(), domain.RemoteInstallRequest{
		Operation: domain.RemoteInstallBootstrapOperation, Host: "pc01", Address: "192.0.2.20",
		Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}, secret)
	response := worker.handle(context.Background(), domain.RemoteInstallRequest{
		Operation: domain.RemoteInstallPrepareOperation, OperationID: bootstrapResponse.OperationID, Host: "pc01",
	}, nil)
	if response.State != "prepared" || response.Session == nil || response.Session.Preparation == nil {
		t.Fatalf("prepare response=%+v", response)
	}
	stored := state.sessions[bootstrapResponse.OperationID]
	if stored.Preparation == nil || stored.Preparation.Cache.PublicKey == "" || stored.State != "prepared" {
		t.Fatalf("stored preparation=%+v", stored)
	}
	cancel := worker.handle(context.Background(), domain.RemoteInstallRequest{Operation: domain.RemoteInstallCancelOperation, OperationID: bootstrapResponse.OperationID}, nil)
	if cancel.State != "cancelled" || !bootstrap.closed || !preparer.discarded || !reservations.reservation.released {
		t.Fatalf("prepared cancel=%+v", cancel)
	}
}

func validWorkerLiveSession() adapters.VerifiedLiveSession {
	key := "ssh-ed25519 YWJjZA== nixorium-session:test"
	return adapters.VerifiedLiveSession{
		HostPublicKey: "ssh-ed25519 YWJjZA== live@test", PublicKeyLine: "restrict " + key,
		PrivateKeyPath: "/run/nixorium/remote-install/test/id_ed25519", KnownHostsPath: "/run/nixorium/remote-install/test/known_hosts",
		Facts: domain.RemoteMachineFacts{
			SchemaVersion: domain.RemoteInstallSchemaVersion, VariantID: "installer", VersionID: "26.05", BuildID: "26.05.test",
			Architecture: "x86_64", UEFI: true, SudoReady: true, BootID: "01234567-89ab-cdef-0123-456789abcdef",
			Interfaces: []domain.RemoteNetworkInterface{{Name: "enp0s2", Addresses: []string{"192.0.2.20"}}}, Disks: []domain.RemoteDisk{},
		},
	}
}

func validWorkerPreparation() domain.RemoteInstallPreparation {
	return domain.RemoteInstallPreparation{
		Repository: "/home/admin/nixorium-deployment", DeploymentRevision: "0123456789abcdef0123456789abcdef01234567",
		BundlePath: "/nix/store/11111111111111111111111111111111-remote-installer", BundleClosureBytes: 1024,
		SystemPath: "/nix/store/22222222222222222222222222222222-nixos-system-pc01-test", SystemClosureBytes: 2048,
		Host:           domain.RemoteInstallHost{Interface: "enp0s2", StaticIP: "192.0.2.101"},
		Cache:          domain.RemoteInstallCache{URL: "http://192.0.2.10:5000", PublicKey: "cache.example:YWJjZA=="},
		AdminPublicKey: "ssh-ed25519 YWJjZA== admin@test", HostKeyPublic: "ssh-ed25519 YWJjZA== live@test",
		HostFingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Facts: validWorkerLiveSession().Facts,
		PreparedAt: time.Unix(1, 0).UTC(), Issues: []domain.ValidationIssue{},
		Endpoints: []domain.RemoteInstallerEndpoint{{Address: "192.0.2.20", Port: 22, Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}},
	}
}
