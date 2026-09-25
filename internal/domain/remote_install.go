package domain

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	RemoteInstallSchemaVersion = 1
	RemoteInstallPlanMaxBytes  = 64 * 1024
	RemoteInstallFactsMaxBytes = 1024 * 1024
	RemoteInstallMaximumDisks  = 128
	RemoteInstallMaximumNICs   = 64
	RemoteInstallReviewWindow  = 10 * time.Minute
)

type RemoteInstallMethod string

const RemoteInstallUSBSSH RemoteInstallMethod = "usb-ssh"

type RemoteInstallReservation interface {
	ReleaseResolved() error
}

type RemoteInstallCapabilities struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Method        RemoteInstallMethod `json:"method"`
	Ready         bool                `json:"ready"`
	Issues        []ValidationIssue   `json:"issues"`
}

type RemoteInstallerEndpoint struct {
	Address     string `json:"address"`
	Port        int    `json:"port"`
	Fingerprint string `json:"fingerprint"`
}

type RemoteNetworkInterface struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
}

type RemoteDisk struct {
	Path             string   `json:"path"`
	KName            string   `json:"kname"`
	MajorMinor       string   `json:"majorMinor"`
	SizeBytes        uint64   `json:"sizeBytes"`
	Serial           string   `json:"serial"`
	WWN              string   `json:"wwn"`
	Model            string   `json:"model"`
	Transport        string   `json:"transport"`
	DiskSeq          string   `json:"diskSeq"`
	Eligible         bool     `json:"eligible,omitempty"`
	ExclusionReasons []string `json:"exclusionReasons,omitempty"`
}

type RemoteMachineFacts struct {
	SchemaVersion        int                      `json:"schemaVersion"`
	VariantID            string                   `json:"variantId"`
	VersionID            string                   `json:"versionId"`
	BuildID              string                   `json:"buildId"`
	Architecture         string                   `json:"architecture"`
	UEFI                 bool                     `json:"uefi"`
	SudoReady            bool                     `json:"sudoReady"`
	BootID               string                   `json:"bootId"`
	MemoryAvailableBytes uint64                   `json:"memoryAvailableBytes"`
	StoreAvailableBytes  uint64                   `json:"storeAvailableBytes"`
	Interfaces           []RemoteNetworkInterface `json:"interfaces"`
	Disks                []RemoteDisk             `json:"disks"`
}

type RemoteInstallHost struct {
	Name      string `json:"name"`
	Interface string `json:"interface"`
	LiveIP    string `json:"liveIp"`
	StaticIP  string `json:"staticIp"`
}

type RemoteInstallCache struct {
	URL       string `json:"url"`
	PublicKey string `json:"publicKey"`
}

// RemoteInstallArtifacts is the target-independent, GC-rooted preparation
// produced before a live installer is connected. It intentionally contains no
// observed endpoint, disk inventory, host key, or review decision.
type RemoteInstallArtifacts struct {
	SchemaVersion      int               `json:"schemaVersion"`
	OperationID        string            `json:"operationId"`
	Repository         string            `json:"repository"`
	DeploymentRevision string            `json:"deploymentRevision"`
	BundlePath         string            `json:"bundlePath"`
	BundleClosureBytes uint64            `json:"bundleClosureBytes"`
	SystemPath         string            `json:"systemPath"`
	SystemClosureBytes uint64            `json:"systemClosureBytes"`
	HostName           string            `json:"hostName"`
	HostInterface      string            `json:"hostInterface"`
	HostStaticIP       string            `json:"hostStaticIp"`
	CachePublicKey     string            `json:"cachePublicKey"`
	AdminPublicKey     string            `json:"adminPublicKey"`
	PreparedAt         time.Time         `json:"preparedAt"`
	Issues             []ValidationIssue `json:"issues"`
}

// RemoteInstallPlan is the exact non-secret payload consumed by the immutable
// remote helper. Passwords and private keys intentionally have no field here.
type RemoteInstallPlan struct {
	SchemaVersion      int                `json:"schemaVersion"`
	OperationID        string             `json:"operationId"`
	BootID             string             `json:"bootId"`
	DeploymentRevision string             `json:"deploymentRevision"`
	SystemPath         string             `json:"systemPath"`
	Host               RemoteInstallHost  `json:"host"`
	Cache              RemoteInstallCache `json:"cache"`
	Disk               RemoteDisk         `json:"disk"`
	AdminPublicKey     string             `json:"adminPublicKey"`
	HostKeyPublic      string             `json:"hostKeyPublic"`
	HostKeyRotation    bool               `json:"hostKeyRotation"`
}

type RemoteInstallPreparation struct {
	OperationID        string                    `json:"operationId"`
	Repository         string                    `json:"repository"`
	DeploymentRevision string                    `json:"deploymentRevision"`
	BundlePath         string                    `json:"bundlePath"`
	BundleClosureBytes uint64                    `json:"bundleClosureBytes"`
	SystemPath         string                    `json:"systemPath"`
	SystemClosureBytes uint64                    `json:"systemClosureBytes"`
	Host               RemoteInstallHost         `json:"host"`
	Cache              RemoteInstallCache        `json:"cache"`
	AdminPublicKey     string                    `json:"adminPublicKey"`
	HostKeyPublic      string                    `json:"hostKeyPublic"`
	HostFingerprint    string                    `json:"hostFingerprint"`
	KnownHostConflict  bool                      `json:"knownHostConflict"`
	Facts              RemoteMachineFacts        `json:"facts"`
	PreparedAt         time.Time                 `json:"preparedAt"`
	Issues             []ValidationIssue         `json:"issues"`
	Endpoints          []RemoteInstallerEndpoint `json:"endpoints,omitempty"`
}

type RemoteInstallPlanReport struct {
	SchemaVersion   int                      `json:"schemaVersion"`
	Operation       string                   `json:"operation"`
	State           string                   `json:"state"`
	Repository      string                   `json:"repository"`
	Method          RemoteInstallMethod      `json:"method"`
	OperationID     string                   `json:"operationId"`
	Host            RemoteInstallHost        `json:"host"`
	Disk            RemoteDisk               `json:"disk"`
	Revision        string                   `json:"revision"`
	BundlePath      string                   `json:"bundlePath"`
	SystemPath      string                   `json:"systemPath"`
	CacheURL        string                   `json:"cacheUrl"`
	HostKeyRotation bool                     `json:"hostKeyRotation"`
	ExpiresAt       time.Time                `json:"expiresAt,omitempty"`
	ReviewToken     string                   `json:"reviewToken,omitempty"`
	Confirmation    string                   `json:"confirmation,omitempty"`
	Message         string                   `json:"message,omitempty"`
	Issues          []ValidationIssue        `json:"issues"`
	Plan            RemoteInstallPlan        `json:"-"`
	Preparation     RemoteInstallPreparation `json:"-"`
}

func (r RemoteInstallPlanReport) HasErrors() bool {
	return r.State != "ready" || len(r.Issues) > 0
}

type RemoteInstallPhase string

const (
	RemoteInstallPhasePreflight              RemoteInstallPhase = "preflight"
	RemoteInstallPhasePrepare                RemoteInstallPhase = "prepare"
	RemoteInstallPhaseTransfer               RemoteInstallPhase = "transfer"
	RemoteInstallPhaseProbe                  RemoteInstallPhase = "probe"
	RemoteInstallPhaseReview                 RemoteInstallPhase = "review"
	RemoteInstallPhaseRevalidate             RemoteInstallPhase = "revalidate"
	RemoteInstallPhasePartition              RemoteInstallPhase = "partition"
	RemoteInstallPhaseInstall                RemoteInstallPhase = "install"
	RemoteInstallPhaseVerify                 RemoteInstallPhase = "verify"
	RemoteInstallPhaseReadyToReboot          RemoteInstallPhase = "ready-to-reboot"
	RemoteInstallPhaseReboot                 RemoteInstallPhase = "reboot"
	RemoteInstallPhasePostBootVerify         RemoteInstallPhase = "post-boot-verify"
	RemoteInstallPhaseReconciliationRequired RemoteInstallPhase = "reconciliation-required"
)

type RemoteInstallProgress struct {
	Phase   RemoteInstallPhase `json:"phase"`
	Detail  string             `json:"detail"`
	Current uint64             `json:"current"`
	Total   uint64             `json:"total"`
}

type RemoteInstallReceipt struct {
	SchemaVersion     int                `json:"schemaVersion"`
	OperationID       string             `json:"operationId"`
	State             string             `json:"state"`
	Phase             RemoteInstallPhase `json:"phase"`
	Sequence          uint64             `json:"sequence,omitempty"`
	MutationStarted   bool               `json:"mutationStarted"`
	DiskMayBeModified bool               `json:"diskMayBeModified"`
	Installed         bool               `json:"installed"`
	Message           string             `json:"message,omitempty"`
}

type RemoteInstallExecutionReport struct {
	SchemaVersion      int                `json:"schemaVersion"`
	Operation          string             `json:"operation"`
	State              string             `json:"state"`
	OperationID        string             `json:"operationId"`
	Phase              RemoteInstallPhase `json:"phase"`
	MutationStarted    bool               `json:"mutationStarted"`
	DiskMayBeModified  bool               `json:"diskMayBeModified"`
	Installed          bool               `json:"installed"`
	RebootRequested    bool               `json:"rebootRequested"`
	BootVerified       bool               `json:"bootVerified"`
	DispatchUncertain  bool               `json:"dispatchUncertain"`
	CleanupUnconfirmed bool               `json:"cleanupUnconfirmed"`
	LogID              string             `json:"logId,omitempty"`
	Message            string             `json:"message,omitempty"`
	Issues             []ValidationIssue  `json:"issues"`
}

func (r RemoteInstallExecutionReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || r.State == "reconciliation-required" || len(r.Issues) > 0
}

type RemoteInstallSession struct {
	SchemaVersion     int                           `json:"schemaVersion"`
	OperationID       string                        `json:"operationId"`
	LogID             string                        `json:"logId,omitempty"`
	State             string                        `json:"state"`
	Plan              RemoteInstallPlan             `json:"plan"`
	Artifacts         *RemoteInstallArtifacts       `json:"artifacts,omitempty"`
	Preparation       *RemoteInstallPreparation     `json:"preparation,omitempty"`
	ReviewTokenDigest string                        `json:"reviewTokenDigest,omitempty"`
	ReviewExpiresAt   time.Time                     `json:"reviewExpiresAt,omitempty"`
	TokenConsumed     bool                          `json:"tokenConsumed"`
	DispatchUncertain bool                          `json:"dispatchUncertain"`
	RebootRequested   bool                          `json:"rebootRequested"`
	BootVerified      bool                          `json:"bootVerified"`
	Receipt           *RemoteInstallReceipt         `json:"receipt,omitempty"`
	Events            []RemoteInstallProgress       `json:"events"`
	Bootstrap         *RemoteInstallBootstrapRecord `json:"bootstrap,omitempty"`
}

type RemoteInstallBootstrapRecord struct {
	Host              string             `json:"host"`
	Address           string             `json:"address"`
	HostPublicKey     string             `json:"hostPublicKey"`
	HostFingerprint   string             `json:"hostFingerprint"`
	AuthorizedKeyLine string             `json:"authorizedKeyLine"`
	Facts             RemoteMachineFacts `json:"facts"`
}

// RemoteInstallDiskCompleted distinguishes durable completion of disk work
// from a still-pending reboot or post-boot identity check. It never means that
// the installed host has passed that final identity check.
func RemoteInstallDiskCompleted(session RemoteInstallSession) bool {
	if !session.TokenConsumed || session.Preparation == nil || session.Receipt == nil ||
		session.OperationID != session.Plan.OperationID || session.OperationID != session.Preparation.OperationID ||
		session.OperationID != session.Receipt.OperationID || ValidateRemoteInstallPlan(session.Plan) != nil ||
		ValidateRemoteInstallPreparation(*session.Preparation) != nil {
		return false
	}
	p, plan, receipt := session.Preparation, session.Plan, session.Receipt
	if p.SystemPath != plan.SystemPath || p.DeploymentRevision != plan.DeploymentRevision ||
		p.Host.Name != plan.Host.Name || p.Host.StaticIP != plan.Host.StaticIP ||
		p.HostKeyPublic != plan.HostKeyPublic || p.Facts.BootID != plan.BootID ||
		receipt.SchemaVersion != RemoteInstallSchemaVersion || !receipt.Installed ||
		!receipt.MutationStarted || !receipt.DiskMayBeModified {
		return false
	}
	switch session.State {
	case "ready-to-reboot", "reboot-dispatching", "reboot-requested", "reconciliation-required", "verified":
	default:
		return false
	}
	return (receipt.State == "ready-to-reboot" && receipt.Phase == RemoteInstallPhaseReadyToReboot) ||
		(receipt.State == "reboot-requested" && receipt.Phase == RemoteInstallPhaseReboot && session.RebootRequested)
}

var (
	remoteOperationIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	remoteBootIDPattern      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	remoteHostPattern        = regexp.MustCompile(`^pc[0-9]{2}$`)
	remoteInterfacePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)
	remoteDevicePattern      = regexp.MustCompile(`^/dev/[A-Za-z0-9_.+-]+$`)
	remoteKNamePattern       = regexp.MustCompile(`^[A-Za-z0-9_.+-]+$`)
	remoteMajorMinorPattern  = regexp.MustCompile(`^[0-9]+:[0-9]+$`)
	remoteFingerprintPattern = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{43}$`)
	remotePublicKeyPattern   = regexp.MustCompile(`^ssh-ed25519 [A-Za-z0-9+/]+={0,2}(?: [^\r\n]{1,256})?$`)
	remoteCacheKeyPattern    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}:[A-Za-z0-9+/]+={0,2}$`)
)

func NewRemoteOperationID() (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", fmt.Errorf("generate remote operation ID: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func DecodeRemoteInstallPlan(data []byte) (RemoteInstallPlan, error) {
	var plan RemoteInstallPlan
	if err := decodeRemoteJSON(data, RemoteInstallPlanMaxBytes, &plan); err != nil {
		return plan, fmt.Errorf("decode remote installation plan: %w", err)
	}
	if err := ValidateRemoteInstallPlan(plan); err != nil {
		return plan, err
	}
	return plan, nil
}

func DecodeRemoteMachineFacts(data []byte) (RemoteMachineFacts, error) {
	var facts RemoteMachineFacts
	if err := decodeRemoteJSON(data, RemoteInstallFactsMaxBytes, &facts); err != nil {
		return facts, fmt.Errorf("decode remote machine facts: %w", err)
	}
	if err := ValidateRemoteMachineFacts(facts); err != nil {
		return facts, err
	}
	return facts, nil
}

// ValidateStrictRemoteJSON applies the bounded duplicate-key and trailing-data
// checks used by the remote protocol before an adapter decodes tool-specific
// JSON whose complete schema is owned by that tool.
func ValidateStrictRemoteJSON(data []byte, maximum int) error {
	if len(data) == 0 || len(data) > maximum {
		return fmt.Errorf("JSON size must be between 1 and %d bytes", maximum)
	}
	if !utf8.Valid(data) {
		return errors.New("JSON is not valid UTF-8")
	}
	return rejectDuplicateJSONKeys(data)
}

func DecodeRemoteInstallReceipt(data []byte) (RemoteInstallReceipt, error) {
	var receipt RemoteInstallReceipt
	if err := decodeRemoteJSON(data, RemoteInstallPlanMaxBytes, &receipt); err != nil {
		return receipt, fmt.Errorf("decode remote installation receipt: %w", err)
	}
	if receipt.SchemaVersion != RemoteInstallSchemaVersion || !remoteOperationIDPattern.MatchString(receipt.OperationID) {
		return receipt, errors.New("remote installation receipt identity is invalid")
	}
	if receipt.DiskMayBeModified != receipt.MutationStarted {
		return receipt, errors.New("remote installation receipt has inconsistent disk risk")
	}
	validState := map[string]bool{"unknown": true, "accepted": true, "running": true, "failed": true, "ready-to-reboot": true, "reboot-requested": true}
	validPhase := map[RemoteInstallPhase]bool{
		RemoteInstallPhasePreflight: true, RemoteInstallPhaseRevalidate: true, RemoteInstallPhasePartition: true,
		RemoteInstallPhaseInstall: true, RemoteInstallPhaseVerify: true, RemoteInstallPhaseReadyToReboot: true,
		RemoteInstallPhaseReboot: true, RemoteInstallPhaseReconciliationRequired: true,
	}
	if !validState[receipt.State] || !validPhase[receipt.Phase] || len(receipt.Message) > 4096 {
		return receipt, errors.New("remote installation receipt state is invalid")
	}
	if receipt.Installed && !receipt.MutationStarted {
		return receipt, errors.New("installed receipt must report disk mutation")
	}
	if receipt.State == "ready-to-reboot" && (!receipt.Installed || receipt.Phase != RemoteInstallPhaseReadyToReboot) {
		return receipt, errors.New("ready-to-reboot receipt is inconsistent")
	}
	return receipt, nil
}

func ValidateRemoteInstallPlan(plan RemoteInstallPlan) error {
	if plan.SchemaVersion != RemoteInstallSchemaVersion {
		return fmt.Errorf("unsupported remote installation schema %d", plan.SchemaVersion)
	}
	if !remoteOperationIDPattern.MatchString(plan.OperationID) {
		return errors.New("remote installation operation ID is invalid")
	}
	if !remoteBootIDPattern.MatchString(plan.BootID) {
		return errors.New("remote installation boot ID is invalid")
	}
	if !gitRevisionPattern.MatchString(plan.DeploymentRevision) {
		return errors.New("remote installation revision is not a full Git object ID")
	}
	if !validStorePath(plan.SystemPath) || !strings.Contains(plan.SystemPath, "-nixos-system-"+plan.Host.Name+"-") {
		return errors.New("remote installation system path does not match the host")
	}
	if err := validateRemoteHost(plan.Host); err != nil {
		return err
	}
	if err := validateRemoteCache(plan.Cache); err != nil {
		return err
	}
	if err := validateRemoteDisk(plan.Disk); err != nil {
		return err
	}
	if !remotePublicKeyPattern.MatchString(plan.AdminPublicKey) || !remotePublicKeyPattern.MatchString(plan.HostKeyPublic) {
		return errors.New("remote installation requires canonical Ed25519 public keys")
	}
	return nil
}

func ValidateRemoteInstallArtifacts(artifacts RemoteInstallArtifacts) error {
	if artifacts.SchemaVersion != RemoteInstallSchemaVersion || !remoteOperationIDPattern.MatchString(artifacts.OperationID) {
		return errors.New("remote installation artifact identity is invalid")
	}
	if !filepath.IsAbs(artifacts.Repository) || filepath.Clean(artifacts.Repository) != artifacts.Repository {
		return errors.New("remote installation artifact repository is not canonical")
	}
	if !gitRevisionPattern.MatchString(artifacts.DeploymentRevision) {
		return errors.New("remote installation artifact revision is invalid")
	}
	if !validStorePath(artifacts.BundlePath) || !validStorePath(artifacts.SystemPath) ||
		!strings.Contains(artifacts.SystemPath, "-nixos-system-"+artifacts.HostName+"-") {
		return errors.New("remote installation artifact store paths are invalid")
	}
	if artifacts.BundleClosureBytes == 0 || artifacts.SystemClosureBytes == 0 || artifacts.PreparedAt.IsZero() {
		return errors.New("remote installation artifact measurements are incomplete")
	}
	if !remoteHostPattern.MatchString(artifacts.HostName) || !remoteInterfacePattern.MatchString(artifacts.HostInterface) || validateRemoteIPv4(artifacts.HostStaticIP) != nil {
		return errors.New("remote installation artifact host is invalid")
	}
	if !remoteCacheKeyPattern.MatchString(artifacts.CachePublicKey) || !remotePublicKeyPattern.MatchString(artifacts.AdminPublicKey) {
		return errors.New("remote installation artifact public keys are invalid")
	}
	if len(artifacts.Issues) != 0 {
		return errors.New("remote installation artifacts contain unresolved issues")
	}
	return nil
}

func ValidateRemoteMachineFacts(facts RemoteMachineFacts) error {
	if facts.SchemaVersion != RemoteInstallSchemaVersion {
		return fmt.Errorf("unsupported remote machine facts schema %d", facts.SchemaVersion)
	}
	if facts.VariantID != "installer" || !strings.HasPrefix(facts.VersionID, "26.05") || facts.Architecture != "x86_64" || !facts.UEFI || !facts.SudoReady {
		return errors.New("target is not the supported NixOS 26.05 x86_64 UEFI installer")
	}
	if !remoteBootIDPattern.MatchString(facts.BootID) {
		return errors.New("remote machine boot ID is invalid")
	}
	if len(facts.Interfaces) > RemoteInstallMaximumNICs || len(facts.Disks) > RemoteInstallMaximumDisks {
		return errors.New("remote machine inventory exceeds protocol limits")
	}
	interfaceNames := map[string]bool{}
	for _, networkInterface := range facts.Interfaces {
		if !remoteInterfacePattern.MatchString(networkInterface.Name) || interfaceNames[networkInterface.Name] || len(networkInterface.Addresses) > 64 {
			return errors.New("remote machine interface inventory is invalid")
		}
		interfaceNames[networkInterface.Name] = true
		for _, address := range networkInterface.Addresses {
			if err := validateRemoteIPv4(address); err != nil {
				return fmt.Errorf("remote machine address: %w", err)
			}
		}
	}
	diskPaths := map[string]bool{}
	for _, disk := range facts.Disks {
		if diskPaths[disk.Path] {
			return errors.New("remote machine disk inventory contains duplicate paths")
		}
		diskPaths[disk.Path] = true
		if err := validateRemoteDisk(disk); err != nil {
			return err
		}
		if disk.Eligible != (len(disk.ExclusionReasons) == 0) {
			return errors.New("remote disk eligibility disagrees with exclusion reasons")
		}
	}
	return nil
}

func ValidateRemoteInstallPreparation(preparation RemoteInstallPreparation) error {
	if !remoteOperationIDPattern.MatchString(preparation.OperationID) {
		return errors.New("remote installation preparation identity is invalid")
	}
	if !filepath.IsAbs(preparation.Repository) || filepath.Clean(preparation.Repository) != preparation.Repository {
		return errors.New("remote installation preparation repository is not canonical")
	}
	if !gitRevisionPattern.MatchString(preparation.DeploymentRevision) {
		return errors.New("remote installation preparation revision is invalid")
	}
	if !validStorePath(preparation.BundlePath) || !validStorePath(preparation.SystemPath) {
		return errors.New("remote installation preparation store paths are invalid")
	}
	if preparation.BundleClosureBytes == 0 || preparation.SystemClosureBytes == 0 || preparation.PreparedAt.IsZero() {
		return errors.New("remote installation preparation measurements are incomplete")
	}
	if err := validateRemoteHost(preparation.Host); err != nil {
		return err
	}
	if !strings.Contains(preparation.SystemPath, "-nixos-system-"+preparation.Host.Name+"-") {
		return errors.New("remote installation preparation system does not match the host")
	}
	if err := validateRemoteCache(preparation.Cache); err != nil {
		return err
	}
	if !remotePublicKeyPattern.MatchString(preparation.AdminPublicKey) ||
		!remotePublicKeyPattern.MatchString(preparation.HostKeyPublic) ||
		!remoteFingerprintPattern.MatchString(preparation.HostFingerprint) {
		return errors.New("remote installation preparation keys are invalid")
	}
	if err := ValidateRemoteMachineFacts(preparation.Facts); err != nil {
		return err
	}
	if len(preparation.Issues) != 0 || len(preparation.Endpoints) > RemoteInstallMaximumNICs {
		return errors.New("remote installation preparation contains unresolved or excessive data")
	}
	for _, endpoint := range preparation.Endpoints {
		if err := validateRemoteIPv4(endpoint.Address); err != nil || endpoint.Port < 1 || endpoint.Port > 65535 ||
			!remoteFingerprintPattern.MatchString(endpoint.Fingerprint) {
			return errors.New("remote installation preparation endpoint is invalid")
		}
	}
	return nil
}

func RemoteInstallReviewToken(report RemoteInstallPlanReport) string {
	bound := struct {
		Repository      string
		Method          RemoteInstallMethod
		OperationID     string
		Plan            RemoteInstallPlan
		BundlePath      string
		HostKeyRotation bool
		ExpiresAt       time.Time
	}{report.Repository, report.Method, report.OperationID, report.Plan, report.BundlePath, report.HostKeyRotation, report.ExpiresAt.UTC()}
	content, _ := json.Marshal(bound)
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func RemoteInstallTokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func decodeRemoteJSON(data []byte, maximum int, destination any) error {
	if len(data) == 0 || len(data) > maximum {
		return fmt.Errorf("JSON size must be between 1 and %d bytes", maximum)
	}
	if !utf8.Valid(data) {
		return errors.New("JSON is not valid UTF-8")
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := consumeRemoteJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON data")
		}
		return err
	}
	return nil
}

func consumeRemoteJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		keys := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if keys[key] {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			keys[key] = true
			if err := consumeRemoteJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("invalid JSON object termination")
		}
	case '[':
		for decoder.More() {
			if err := consumeRemoteJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("invalid JSON array termination")
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	return nil
}

func validateRemoteHost(host RemoteInstallHost) error {
	if !remoteHostPattern.MatchString(host.Name) || !remoteInterfacePattern.MatchString(host.Interface) {
		return errors.New("remote installation host identity is invalid")
	}
	if err := validateRemoteIPv4(host.LiveIP); err != nil {
		return fmt.Errorf("live address: %w", err)
	}
	if err := validateRemoteIPv4(host.StaticIP); err != nil {
		return fmt.Errorf("static address: %w", err)
	}
	if host.LiveIP == host.StaticIP {
		return errors.New("live and static client addresses must differ")
	}
	return nil
}

func validateRemoteIPv4(address string) error {
	parsed := net.ParseIP(address)
	if parsed == nil || parsed.To4() == nil || parsed.String() != address {
		return errors.New("IPv4 address is not canonical")
	}
	if parsed.IsUnspecified() || parsed.IsLoopback() || parsed.IsMulticast() || parsed.IsLinkLocalUnicast() || parsed.Equal(net.IPv4bcast) {
		return errors.New("IPv4 address is not a usable unicast address")
	}
	return nil
}

func validateRemoteCache(cache RemoteInstallCache) error {
	parsed, err := url.Parse(cache.URL)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("remote cache URL must be a plain observed HTTP endpoint")
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return errors.New("remote cache URL must contain an IPv4 address and port")
	}
	if err := validateRemoteIPv4(host); err != nil {
		return fmt.Errorf("remote cache address: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("remote cache port is invalid")
	}
	if !remoteCacheKeyPattern.MatchString(cache.PublicKey) {
		return errors.New("remote cache public key is invalid")
	}
	return nil
}

func ValidateRemoteInstallCache(cache RemoteInstallCache) error {
	return validateRemoteCache(cache)
}

func validateRemoteDisk(disk RemoteDisk) error {
	if !remoteDevicePattern.MatchString(disk.Path) || !remoteKNamePattern.MatchString(disk.KName) || disk.Path != "/dev/"+disk.KName || !remoteMajorMinorPattern.MatchString(disk.MajorMinor) {
		return errors.New("remote disk device identity is invalid")
	}
	if disk.SizeBytes < 8*1024*1024*1024 {
		return errors.New("remote disk is below the minimum capacity")
	}
	for _, value := range []string{disk.Serial, disk.WWN, disk.Model, disk.Transport, disk.DiskSeq} {
		if len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
			return errors.New("remote disk identity contains an invalid string")
		}
	}
	if disk.Serial == "" && disk.WWN == "" && disk.DiskSeq == "" {
		return errors.New("remote disk lacks a continuity identifier")
	}
	allowedReasons := []string{"holders-active", "live-media", "mounted", "not-disk", "read-only", "removable", "swap-active", "too-small"}
	if len(disk.ExclusionReasons) > len(allowedReasons) {
		return errors.New("remote disk contains too many exclusion reasons")
	}
	for index, reason := range disk.ExclusionReasons {
		if !slices.Contains(allowedReasons, reason) || (index > 0 && disk.ExclusionReasons[index-1] >= reason) {
			return errors.New("remote disk exclusion reasons are unknown, duplicate, or unsorted")
		}
	}
	return nil
}

func ValidRemoteFingerprint(value string) bool {
	return remoteFingerprintPattern.MatchString(value)
}

func ValidRemoteInterfaceName(value string) bool {
	return remoteInterfacePattern.MatchString(value)
}

func ValidRemoteHostName(value string) bool {
	return remoteHostPattern.MatchString(value)
}
