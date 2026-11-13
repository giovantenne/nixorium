package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// fileRecoveryReviewLifetime is how long a recovery review stays valid at
// least.
const fileRecoveryReviewLifetime = 10 * time.Minute

// FileRecoverySource reads the laboratory identities and asks a student
// computer's recovery helper over the controller's SSH access.
type FileRecoverySource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	FileRecovery(context.Context, domain.HostMeta, domain.FileRecoveryHelperRequest) (domain.FileRecoveryHelperAnswer, error)
}

// FileRecoveryManager copies a student's files from a saved session back
// onto that computer's desktop. Like locking, it is a classroom aid that
// changes only the student's home, so it does not take the administrative
// operation lock, and it works whether the classroom view is on or off.
type FileRecoveryManager struct {
	source FileRecoverySource
	now    func() time.Time
}

func NewFileRecoveryManager(source FileRecoverySource) *FileRecoveryManager {
	return &FileRecoveryManager{source, time.Now}
}

// computer resolves one student computer by name.
func (m *FileRecoveryManager) computer(ctx context.Context, repository, requested string) (domain.HostMeta, error) {
	if requested == "" || requested == "@lab" {
		return domain.HostMeta{}, errors.New("Choose one student computer.")
	}
	meta, err := m.source.LabMeta(ctx, repository)
	if err != nil {
		return domain.HostMeta{}, err
	}
	selected, _, issues := selectDeploymentTargets(meta.Clients.Hosts, requested)
	if len(issues) > 0 {
		return domain.HostMeta{}, errors.New(issues[0])
	}
	if len(selected) != 1 {
		return domain.HostMeta{}, errors.New("Choose one student computer.")
	}
	host := shutdownHostMeta(selected)[0]
	if host.Name == meta.Controller.Name {
		return domain.HostMeta{}, errors.New("The controller cannot be selected.")
	}
	return host, nil
}

func (m *FileRecoveryManager) ask(ctx context.Context, host domain.HostMeta, request domain.FileRecoveryHelperRequest) (domain.FileRecoveryHelperAnswer, error) {
	answer, err := m.source.FileRecovery(ctx, host, request)
	if err != nil {
		return answer, fmt.Errorf("%s could not be reached: %v.", host.Name, err)
	}
	if answer.Error != "" {
		return answer, errors.New(answer.Error)
	}
	return answer, nil
}

// Sessions lists the saved sessions of one computer, most recent first.
func (m *FileRecoveryManager) Sessions(ctx context.Context, repository, requested string) ([]domain.FileRecoverySession, error) {
	host, err := m.computer(ctx, repository, requested)
	if err != nil {
		return nil, err
	}
	answer, err := m.ask(ctx, host, domain.FileRecoveryHelperRequest{Operation: domain.FileRecoveryHelperSessions})
	if err != nil {
		return nil, err
	}
	if answer.Sessions == nil {
		return []domain.FileRecoverySession{}, nil
	}
	return answer.Sessions, nil
}

// List lists one folder of a saved session.
func (m *FileRecoveryManager) List(ctx context.Context, repository, requested string, session int, folder string) (domain.FileRecoveryListing, error) {
	if err := domain.ValidFileRecoveryPath(folder); err != nil || session < 1 {
		return domain.FileRecoveryListing{}, errors.New("The folder or the saved session is not valid.")
	}
	host, err := m.computer(ctx, repository, requested)
	if err != nil {
		return domain.FileRecoveryListing{}, err
	}
	answer, err := m.ask(ctx, host, domain.FileRecoveryHelperRequest{Operation: domain.FileRecoveryHelperList, Session: session, Path: folder})
	if err != nil {
		return domain.FileRecoveryListing{}, err
	}
	if answer.Listing == nil || answer.Listing.Session != session || answer.Listing.Path != folder {
		return domain.FileRecoveryListing{}, errors.New("The student computer gave an answer that is not understood; run Update computers first.")
	}
	return *answer.Listing, nil
}

// session finds a saved session by number, asking the computer again.
func (m *FileRecoveryManager) session(ctx context.Context, host domain.HostMeta, number int) (domain.FileRecoverySession, error) {
	answer, err := m.ask(ctx, host, domain.FileRecoveryHelperRequest{Operation: domain.FileRecoveryHelperSessions})
	if err != nil {
		return domain.FileRecoverySession{}, err
	}
	for _, saved := range answer.Sessions {
		if saved.Number == number && domain.ValidFileRecoverySessionID(saved.ID) {
			return saved, nil
		}
	}
	return domain.FileRecoverySession{}, errors.New("This saved session no longer exists; the computer may have restarted. Choose the session again.")
}

// Plan reviews copying chosen files of a saved session back to the
// student's desktop.
func (m *FileRecoveryManager) Plan(ctx context.Context, repository, requested string, session int, paths []string) domain.FileRecoveryPlan {
	p := domain.FileRecoveryPlan{SchemaVersion: domain.SchemaVersion, Operation: "file-recovery-plan", State: "blocked", Requested: requested, Session: session, Paths: append([]string{}, paths...), Issues: []domain.ValidationIssue{}}
	fail := func(message string) domain.FileRecoveryPlan {
		p.Message = message
		p.Issues = append(p.Issues, domain.ValidationIssue{Field: "recovery", Message: message})
		return p
	}
	var err error
	p.Repository, err = filepath.Abs(repository)
	if err != nil {
		return fail(err.Error())
	}
	if session < 1 {
		return fail("Choose a saved session.")
	}
	if err := domain.ValidFileRecoveryPaths(paths); err != nil {
		return fail("Choose the files again: " + err.Error() + ".")
	}
	host, err := m.computer(ctx, p.Repository, requested)
	if err != nil {
		return fail(err.Error())
	}
	p.Requested, p.Target = host.Name, domain.FileRecoveryTarget{HostMeta: host}
	saved, err := m.session(ctx, host, session)
	if err != nil {
		return fail(err.Error())
	}
	p.SessionID, p.SavedAt = saved.ID, saved.SavedAt
	p.State = "ready"
	p.ExpiresAt = reviewExpiry(m.now(), fileRecoveryReviewLifetime)
	p.Message = fmt.Sprintf("Copies %d chosen item(s) into the folder “%s” on the desktop of %s. Nothing there is replaced.", len(paths), domain.FileRecoveryFolder, host.Name)
	p.ReviewToken = domain.FileRecoveryReviewToken(p)
	return p
}

// Apply copies the reviewed files. The computer must still be in the
// inventory with the same address and still hold the same saved session.
func (m *FileRecoveryManager) Apply(ctx context.Context, p domain.FileRecoveryPlan, token string) domain.FileRecoveryReport {
	r := domain.FileRecoveryReport{SchemaVersion: domain.SchemaVersion, Operation: "file-recovery-apply", State: "blocked", Computer: p.Target.Name}
	if p.HasErrors() || token == "" || token != p.ReviewToken || token != domain.FileRecoveryReviewToken(p) || !m.now().Before(p.ExpiresAt) {
		r.Message = "Review expired or changed; create a fresh plan."
		return r
	}
	host, err := m.computer(ctx, p.Repository, p.Target.Name)
	if err != nil {
		r.Message = err.Error()
		return r
	}
	if host.Name != p.Target.Name || host.IP != p.Target.IP {
		r.Message = "Client inventory changed; review again."
		return r
	}
	// The computer checks the session's identity again when it copies.
	answer, err := m.ask(ctx, host, domain.FileRecoveryHelperRequest{Operation: domain.FileRecoveryHelperRestore, Session: p.Session, SessionID: p.SessionID, Paths: p.Paths})
	if err != nil {
		r.State, r.Message = "failed", err.Error()
		return r
	}
	if answer.Restored == nil || answer.Restored.Files < 0 {
		r.State, r.Message = "failed", "The student computer gave an answer that is not understood; run Update computers first."
		return r
	}
	r.State, r.Restored = "completed", answer.Restored
	r.Message = fmt.Sprintf("%d file(s) copied into “%s” on %s. They stay until the computer restarts: the student should save them where they keep their work.", answer.Restored.Files, answer.Restored.Folder, host.Name)
	return r
}
