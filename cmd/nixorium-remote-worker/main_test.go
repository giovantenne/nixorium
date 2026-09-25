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
	session  adapters.VerifiedLiveSession
	err      error
	closeErr error
	closed   bool
	trace    *[]string
}

func (bootstrap *fakeWorkerBootstrap) Establish(_ context.Context, operationID, address, fingerprint string, secret *adapters.LivePassword) (adapters.VerifiedLiveSession, error) {
	bootstrap.session.OperationID = operationID
	bootstrap.session.Address = address
	bootstrap.session.HostFingerprint = fingerprint
	return bootstrap.session, bootstrap.err
}

func (bootstrap *fakeWorkerBootstrap) RecoverSession(operationID string, record domain.RemoteInstallBootstrapRecord) (adapters.VerifiedLiveSession, error) {
	if bootstrap.err != nil {
		return adapters.VerifiedLiveSession{}, bootstrap.err
	}
	bootstrap.session.OperationID = operationID
	bootstrap.session.Address = record.Address
	bootstrap.session.HostPublicKey = record.HostPublicKey
	bootstrap.session.HostFingerprint = record.HostFingerprint
	bootstrap.session.PublicKeyLine = record.AuthorizedKeyLine
	bootstrap.session.Facts = record.Facts
	return bootstrap.session, nil
}

func (bootstrap *fakeWorkerBootstrap) CloseSession(context.Context, adapters.VerifiedLiveSession) error {
	bootstrap.closed = true
	if bootstrap.trace != nil {
		*bootstrap.trace = append(*bootstrap.trace, "close")
	}
	return bootstrap.closeErr
}

func (bootstrap *fakeWorkerBootstrap) DiscardLocalSession(adapters.VerifiedLiveSession) error {
	bootstrap.closed = true
	return nil
}

type fakeWorkerReservation struct {
	released bool
	trace    *[]string
}

func (reservation *fakeWorkerReservation) ReleaseResolved() error {
	reservation.released = true
	if reservation.trace != nil {
		*reservation.trace = append(*reservation.trace, "release")
	}
	return nil
}

type fakeWorkerReservations struct {
	reservation *fakeWorkerReservation
	err         error
	recoverID   string
}

type fakeWorkerPreparer struct {
	preparation domain.RemoteInstallPreparation
	artifacts   domain.RemoteInstallArtifacts
	err         error
	discardErr  error
	discarded   bool
	trace       *[]string
}

func (preparer *fakeWorkerPreparer) PrepareArtifacts(_ context.Context, operationID, host string) (domain.RemoteInstallArtifacts, error) {
	preparer.artifacts.OperationID = operationID
	preparer.artifacts.HostName = host
	return preparer.artifacts, preparer.err
}

func (preparer *fakeWorkerPreparer) Finalize(_ context.Context, artifacts domain.RemoteInstallArtifacts, live adapters.VerifiedLiveSession) (domain.RemoteInstallPreparation, error) {
	preparer.preparation.OperationID = artifacts.OperationID
	preparer.preparation.Host.Name = artifacts.HostName
	preparer.preparation.Host.LiveIP = live.Address
	return preparer.preparation, preparer.err
}

func (preparer *fakeWorkerPreparer) Discard(string) error {
	preparer.discarded = true
	if preparer.trace != nil {
		*preparer.trace = append(*preparer.trace, "discard")
	}
	return preparer.discardErr
}

type fakeWorkerStatusConnection struct {
	receipt   domain.RemoteInstallReceipt
	statusErr error
	log       []byte
	logErr    error
	trace     *[]string
}

func (connection *fakeWorkerStatusConnection) Status(context.Context, string, string) (domain.RemoteInstallReceipt, error) {
	if connection.trace != nil {
		*connection.trace = append(*connection.trace, "status")
	}
	return connection.receipt, connection.statusErr
}

func (connection *fakeWorkerStatusConnection) OperationLog(context.Context, string, string) ([]byte, error) {
	if connection.trace != nil {
		*connection.trace = append(*connection.trace, "log")
	}
	return connection.log, connection.logErr
}

func (source *fakeWorkerReservations) ReserveRemoteSession(string) (domain.RemoteInstallReservation, error) {
	if source.err != nil {
		return nil, source.err
	}
	source.reservation = &fakeWorkerReservation{}
	return source.reservation, nil
}

func (source *fakeWorkerReservations) RecoverRemoteSession() (string, domain.RemoteInstallReservation, bool, error) {
	if source.err != nil {
		return "", nil, false, source.err
	}
	if source.recoverID == "" {
		return "", nil, false, nil
	}
	source.reservation = &fakeWorkerReservation{}
	return source.recoverID, source.reservation, true, nil
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
	preparer := &fakeWorkerPreparer{artifacts: validWorkerArtifacts(), preparation: validWorkerPreparation()}
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

func TestRemoteWorkerConfirmedFailureBeforeMutationCanBeCancelled(t *testing.T) {
	operationID := "0123456789abcdef0123456789abcdef"
	live := validWorkerLiveSession()
	preparation := validWorkerPreparation()
	preparation.OperationID = operationID
	receipt := domain.RemoteInstallReceipt{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		State: "failed", Phase: domain.RemoteInstallPhasePreflight,
	}
	session := domain.RemoteInstallSession{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		State: "failed", TokenConsumed: true, Receipt: &receipt, Preparation: &preparation,
		Bootstrap: &domain.RemoteInstallBootstrapRecord{
			Host: "pc01", Address: "192.0.2.20", HostPublicKey: live.HostPublicKey,
			HostFingerprint:   "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			AuthorizedKeyLine: live.PublicKeyLine, Facts: live.Facts,
		},
		Events: []domain.RemoteInstallProgress{},
	}
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{operationID: session}}
	bootstrap := &fakeWorkerBootstrap{session: live}
	preparer := &fakeWorkerPreparer{}
	reservation := &fakeWorkerReservation{}
	worker := &remoteWorker{
		state: state, bootstrap: bootstrap, preparer: preparer,
		operationID: operationID, liveSession: &live, reservation: reservation,
	}
	response := worker.handle(context.Background(), domain.RemoteInstallRequest{
		Operation: domain.RemoteInstallCancelOperation, OperationID: operationID,
	}, nil)
	if response.State != "cancelled" || !bootstrap.closed || !preparer.discarded || !reservation.released || worker.operationID != "" {
		t.Fatalf("confirmed pre-mutation failure cancellation=%+v worker=%+v", response, worker)
	}

	unsafe := session
	unsafe.Receipt = &domain.RemoteInstallReceipt{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		State: "failed", Phase: domain.RemoteInstallPhasePartition,
		MutationStarted: true, DiskMayBeModified: true,
	}
	if remoteSessionSafelyCancellable(unsafe) {
		t.Fatal("a failure after disk mutation was marked safely cancellable")
	}
}

func TestRemoteWorkerAutomaticallyResolvesConfirmedFailureBeforeMutation(t *testing.T) {
	operationID := "0123456789abcdef0123456789abcdef"
	live := validWorkerLiveSession()
	receipt := domain.RemoteInstallReceipt{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		State: "failed", Phase: domain.RemoteInstallPhasePreflight,
	}
	session := domain.RemoteInstallSession{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		State: "failed", TokenConsumed: true, Receipt: &receipt, Events: []domain.RemoteInstallProgress{},
	}
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{operationID: session}}
	bootstrap := &fakeWorkerBootstrap{session: live}
	preparer := &fakeWorkerPreparer{}
	reservation := &fakeWorkerReservation{}
	worker := &remoteWorker{
		state: state, bootstrap: bootstrap, preparer: preparer,
		operationID: operationID, liveSession: &live, reservation: reservation,
	}
	if err := worker.resolveConfirmedFailureLocked(context.Background(), &session); err != nil {
		t.Fatal(err)
	}
	stored := state.sessions[operationID]
	if stored.State != "failed-resolved" || !bootstrap.closed || !preparer.discarded || !reservation.released ||
		worker.operationID != "" || worker.liveSession != nil || worker.reservation != nil {
		t.Fatalf("resolved=%+v worker=%+v", stored, worker)
	}
}

func TestRemoteWorkerStatusPublishesFailureLogBeforeAutomaticCleanup(t *testing.T) {
	operationID := "0123456789abcdef0123456789abcdef"
	live := validWorkerLiveSession()
	receipt := domain.RemoteInstallReceipt{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		State: "failed", Phase: domain.RemoteInstallPhasePreflight,
	}
	session := domain.RemoteInstallSession{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		LogID: "usb-install-" + operationID + ".log", State: "accepted", TokenConsumed: true,
		Preparation: &domain.RemoteInstallPreparation{BundlePath: "/nix/store/11111111111111111111111111111111-remote-installer"},
		Events:      []domain.RemoteInstallProgress{},
	}
	trace := []string{}
	connection := &fakeWorkerStatusConnection{receipt: receipt, log: []byte("/mnt is already occupied\n"), trace: &trace}
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{operationID: session}}
	bootstrap := &fakeWorkerBootstrap{session: live, trace: &trace}
	preparer := &fakeWorkerPreparer{trace: &trace}
	reservation := &fakeWorkerReservation{trace: &trace}
	published := []byte(nil)
	worker := &remoteWorker{
		state: state, bootstrap: bootstrap, preparer: preparer,
		statusConnection: func(adapters.VerifiedLiveSession) (remoteStatusConnection, error) { return connection, nil },
		publishRemoteLog: func(id string, content []byte, result string) (string, error) {
			trace = append(trace, "publish")
			if id != operationID || result != "failed" {
				t.Fatalf("publish id=%q result=%q", id, result)
			}
			published = append([]byte(nil), content...)
			return "usb-install-" + id + ".log", nil
		},
		operationID: operationID, liveSession: &live, reservation: reservation,
	}
	response := worker.handleStatus(context.Background(), domain.RemoteInstallRequest{OperationID: operationID})
	if response.State != "failed-resolved" || response.Session == nil || response.Session.State != "failed-resolved" ||
		!strings.Contains(response.Message, "controller reservation was released automatically") {
		t.Fatalf("response=%+v", response)
	}
	if string(published) != "/mnt is already occupied\n" || !bootstrap.closed || !preparer.discarded || !reservation.released {
		t.Fatalf("published=%q bootstrap=%+v preparer=%+v reservation=%+v", published, bootstrap, preparer, reservation)
	}
	expectedTrace := []string{"status", "log", "publish", "close", "discard", "release"}
	if strings.Join(trace, ",") != strings.Join(expectedTrace, ",") {
		t.Fatalf("trace=%v want=%v", trace, expectedTrace)
	}
}

func TestRemoteWorkerRecoveryFinishesResolvedFailureRelease(t *testing.T) {
	operationID := "0123456789abcdef0123456789abcdef"
	receipt := domain.RemoteInstallReceipt{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		State: "failed", Phase: domain.RemoteInstallPhasePreflight,
	}
	session := domain.RemoteInstallSession{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		State: "failed-resolved", TokenConsumed: true, Receipt: &receipt, Events: []domain.RemoteInstallProgress{},
	}
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{operationID: session}}
	preparer := &fakeWorkerPreparer{}
	reservations := &fakeWorkerReservations{recoverID: operationID}
	worker := &remoteWorker{
		state: state, bootstrap: &fakeWorkerBootstrap{}, preparer: preparer, reservations: reservations,
	}
	if err := worker.recover(); err != nil {
		t.Fatal(err)
	}
	if !preparer.discarded || reservations.reservation == nil || !reservations.reservation.released ||
		worker.operationID != "" || worker.reservation != nil {
		t.Fatalf("preparer=%+v reservations=%+v worker=%+v", preparer, reservations, worker)
	}
}

func TestRemoteWorkerResolvedFailureDoesNotLetRootCleanupHoldControllerReservation(t *testing.T) {
	operationID := "0123456789abcdef0123456789abcdef"
	receipt := domain.RemoteInstallReceipt{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		State: "failed", Phase: domain.RemoteInstallPhasePreflight,
	}
	session := domain.RemoteInstallSession{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		State: "failed-resolved", Receipt: &receipt, Events: []domain.RemoteInstallProgress{},
	}
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{operationID: session}}
	preparer := &fakeWorkerPreparer{discardErr: errors.New("fixture root cleanup failed")}
	reservation := &fakeWorkerReservation{}
	worker := &remoteWorker{
		state: state, preparer: preparer, operationID: operationID, reservation: reservation,
	}
	if err := worker.releaseResolvedFailureLocked(&session); err != nil {
		t.Fatal(err)
	}
	stored := state.sessions[operationID]
	if !reservation.released || worker.operationID != "" || worker.reservation != nil || len(stored.Events) != 1 ||
		!strings.Contains(stored.Events[0].Detail, "preparation-root cleanup needs maintenance") {
		t.Fatalf("stored=%+v worker=%+v reservation=%+v", stored, worker, reservation)
	}
}

func TestRemoteWorkerRetainsReservationWhenLiveKeyRevocationFails(t *testing.T) {
	operationID := "0123456789abcdef0123456789abcdef"
	live := validWorkerLiveSession()
	receipt := domain.RemoteInstallReceipt{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		State: "failed", Phase: domain.RemoteInstallPhasePreflight,
	}
	session := domain.RemoteInstallSession{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		State: "failed", TokenConsumed: true, Receipt: &receipt, Events: []domain.RemoteInstallProgress{},
	}
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{operationID: session}}
	bootstrap := &fakeWorkerBootstrap{session: live, closeErr: errors.New("fixture revoke failed")}
	reservation := &fakeWorkerReservation{}
	worker := &remoteWorker{
		state: state, bootstrap: bootstrap, preparer: &fakeWorkerPreparer{},
		operationID: operationID, liveSession: &live, reservation: reservation,
	}
	if err := worker.resolveConfirmedFailureLocked(context.Background(), &session); err == nil {
		t.Fatal("expected live key revocation failure")
	}
	if reservation.released || worker.operationID != operationID || worker.liveSession == nil || worker.reservation == nil ||
		state.sessions[operationID].State != "failed" {
		t.Fatalf("state=%+v worker=%+v reservation=%+v", state.sessions[operationID], worker, reservation)
	}
}

func TestRemoteWorkerPreparesArtifactsBeforeTargetAndReusesThem(t *testing.T) {
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{}}
	bootstrap := &fakeWorkerBootstrap{session: validWorkerLiveSession()}
	reservations := &fakeWorkerReservations{}
	preparer := &fakeWorkerPreparer{artifacts: validWorkerArtifacts(), preparation: validWorkerPreparation()}
	worker := &remoteWorker{state: state, bootstrap: bootstrap, preparer: preparer, reservations: reservations}

	prepared := worker.handle(context.Background(), domain.RemoteInstallRequest{Operation: domain.RemoteInstallPrepareOperation, Host: "pc01"}, nil)
	if prepared.State != "artifacts-ready" || prepared.OperationID == "" || prepared.Session == nil || prepared.Session.Artifacts == nil || prepared.Session.Bootstrap != nil {
		t.Fatalf("artifact preparation=%+v", prepared)
	}
	probe := worker.handle(context.Background(), domain.RemoteInstallRequest{Operation: domain.RemoteInstallWorkerProbeOperation}, nil)
	if probe.OperationID != prepared.OperationID || probe.State != "artifacts-ready" || probe.Session == nil {
		t.Fatalf("worker probe did not expose its resumable operation: %+v", probe)
	}
	secret, _ := adapters.NewLivePassword([]byte("temporary-secret"))
	bootstrapped := worker.handle(context.Background(), domain.RemoteInstallRequest{
		Operation: domain.RemoteInstallBootstrapOperation, Host: "pc01", Address: "192.0.2.20",
		Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}, secret)
	if bootstrapped.OperationID != prepared.OperationID || bootstrapped.State != "bootstrapped-artifacts" || bootstrapped.Session.Artifacts == nil {
		t.Fatalf("prepared artifacts were not reused: prepared=%+v bootstrap=%+v", prepared, bootstrapped)
	}
	cancelled := worker.handle(context.Background(), domain.RemoteInstallRequest{Operation: domain.RemoteInstallCancelOperation, OperationID: prepared.OperationID}, nil)
	if cancelled.State != "cancelled" || !preparer.discarded || !bootstrap.closed || !reservations.reservation.released {
		t.Fatalf("prepared session cleanup=%+v", cancelled)
	}
}

func TestRemoteWorkerRecoversReservedLiveSessionWithoutReplayingDispatch(t *testing.T) {
	operationID := "0123456789abcdef0123456789abcdef"
	live := validWorkerLiveSession()
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{
		operationID: {
			SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
			LogID: "usb-install-" + operationID + ".log", State: "dispatching", TokenConsumed: true,
			Bootstrap: &domain.RemoteInstallBootstrapRecord{
				Host: "pc01", Address: "192.0.2.20", HostPublicKey: live.HostPublicKey,
				HostFingerprint:   "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
				AuthorizedKeyLine: live.PublicKeyLine, Facts: live.Facts,
			},
			Events: []domain.RemoteInstallProgress{},
		},
	}}
	bootstrap := &fakeWorkerBootstrap{session: live}
	reservations := &fakeWorkerReservations{recoverID: operationID}
	worker := &remoteWorker{state: state, bootstrap: bootstrap, reservations: reservations}
	if err := worker.recover(); err != nil {
		t.Fatal(err)
	}
	stored := state.sessions[operationID]
	if worker.operationID != operationID || worker.reservation == nil || worker.liveSession == nil ||
		stored.State != "reconciliation-required" || !stored.DispatchUncertain || len(stored.Events) != 1 {
		t.Fatalf("worker=%+v stored=%+v", worker, stored)
	}
	probe := worker.handle(context.Background(), domain.RemoteInstallRequest{Operation: domain.RemoteInstallWorkerProbeOperation}, nil)
	if probe.OperationID != operationID || probe.State != "reconciliation-required" {
		t.Fatalf("recovered probe=%+v", probe)
	}
}

func TestRemoteWorkerRecoveryWithoutRuntimeCredentialFailsClosed(t *testing.T) {
	operationID := "0123456789abcdef0123456789abcdef"
	live := validWorkerLiveSession()
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{
		operationID: {
			SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID, State: "prepared",
			Bootstrap: &domain.RemoteInstallBootstrapRecord{
				Host: "pc01", Address: "192.0.2.20", HostPublicKey: live.HostPublicKey,
				HostFingerprint:   "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
				AuthorizedKeyLine: live.PublicKeyLine, Facts: live.Facts,
			},
			Events: []domain.RemoteInstallProgress{},
		},
	}}
	worker := &remoteWorker{
		state: state, bootstrap: &fakeWorkerBootstrap{err: errors.New("runtime key missing")},
		reservations: &fakeWorkerReservations{recoverID: operationID},
	}
	if err := worker.recover(); err != nil {
		t.Fatal(err)
	}
	stored := state.sessions[operationID]
	if worker.reservation == nil || worker.liveSession != nil || stored.State != "reconciliation-required" {
		t.Fatalf("worker=%+v stored=%+v", worker, stored)
	}
	blockedCancel := worker.handle(context.Background(), domain.RemoteInstallRequest{Operation: domain.RemoteInstallCancelOperation, OperationID: operationID}, nil)
	if blockedCancel.State != "reconciliation-required" || worker.reservation == nil {
		t.Fatalf("unverified recovery cancellation=%+v worker=%+v", blockedCancel, worker)
	}
	bootstrap := worker.bootstrap.(*fakeWorkerBootstrap)
	bootstrap.err = nil
	bootstrap.session = live
	secret, _ := adapters.NewLivePassword([]byte("new-console-password"))
	response := worker.handle(context.Background(), domain.RemoteInstallRequest{
		Operation: domain.RemoteInstallBootstrapOperation, Host: "pc01", Address: "192.0.2.20",
		Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}, secret)
	if response.State != "recovery-attached" || worker.liveSession == nil || state.sessions[operationID].State != "bootstrapped" {
		t.Fatalf("recovery bootstrap response=%+v worker=%+v", response, worker)
	}
	cancelled := worker.handle(context.Background(), domain.RemoteInstallRequest{Operation: domain.RemoteInstallCancelOperation, OperationID: operationID}, nil)
	if cancelled.State != "cancelled" || !bootstrap.closed || !worker.reservations.(*fakeWorkerReservations).reservation.released || worker.operationID != "" {
		t.Fatalf("verified recovery cancellation=%+v worker=%+v", cancelled, worker)
	}
}

func TestRemoteWorkerRecoveryRejectsDifferentBootWithoutReplaying(t *testing.T) {
	operationID := "0123456789abcdef0123456789abcdef"
	live := validWorkerLiveSession()
	oldBoot := live.Facts.BootID
	state := &fakeWorkerState{sessions: map[string]domain.RemoteInstallSession{
		operationID: {
			SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
			State: "reconciliation-required", TokenConsumed: true, DispatchUncertain: true,
			Bootstrap: &domain.RemoteInstallBootstrapRecord{
				Host: "pc01", Address: "192.0.2.20", HostPublicKey: live.HostPublicKey,
				HostFingerprint:   "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
				AuthorizedKeyLine: live.PublicKeyLine, Facts: live.Facts,
			}, Events: []domain.RemoteInstallProgress{},
		},
	}}
	live.Facts.BootID = "ffffffff-ffff-ffff-ffff-ffffffffffff"
	bootstrap := &fakeWorkerBootstrap{session: live, err: errors.New("runtime key missing")}
	worker := &remoteWorker{
		state: state, bootstrap: bootstrap, reservations: &fakeWorkerReservations{recoverID: operationID},
	}
	if err := worker.recover(); err != nil {
		t.Fatal(err)
	}
	bootstrap.err = nil
	secret, _ := adapters.NewLivePassword([]byte("new-console-password"))
	response := worker.handle(context.Background(), domain.RemoteInstallRequest{
		Operation: domain.RemoteInstallBootstrapOperation, Host: "pc01", Address: "192.0.2.20",
		Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}, secret)
	if response.State != "reconciliation-required" || !strings.Contains(response.Message, "boot ID changed") || !bootstrap.closed || worker.liveSession != nil {
		t.Fatalf("changed-boot response=%+v worker=%+v", response, worker)
	}
	if state.sessions[operationID].Bootstrap.Facts.BootID != oldBoot || !state.sessions[operationID].TokenConsumed {
		t.Fatal("changed boot replaced or reauthorized the persisted operation")
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

func validWorkerArtifacts() domain.RemoteInstallArtifacts {
	return domain.RemoteInstallArtifacts{
		SchemaVersion: domain.RemoteInstallSchemaVersion, Repository: "/home/admin/nixorium-deployment",
		DeploymentRevision: "0123456789abcdef0123456789abcdef01234567",
		BundlePath:         "/nix/store/11111111111111111111111111111111-remote-installer", BundleClosureBytes: 1024,
		SystemPath: "/nix/store/22222222222222222222222222222222-nixos-system-pc01-test", SystemClosureBytes: 2048,
		HostInterface: "enp0s2", HostStaticIP: "192.0.2.101", CachePublicKey: "cache.example:YWJjZA==",
		AdminPublicKey: "ssh-ed25519 YWJjZA== admin@test", PreparedAt: time.Unix(1, 0).UTC(), Issues: []domain.ValidationIssue{},
	}
}
