package presentation

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// DemoGraphBundle holds navigable website demos. Each graph is explored from
// the real dashboard model by pressing navigation keys against synthetic
// actions; the website follows recorded edges instead of reimplementing the
// TUI. Destructive steps stop at their typed confirmation because the
// explorer never types confirmation words.
type DemoGraphBundle struct {
	SchemaVersion int         `json:"schemaVersion"`
	SourceCommit  string      `json:"sourceCommit"`
	SourceDate    string      `json:"sourceDate"`
	Terminal      string      `json:"terminal"`
	Synthetic     bool        `json:"synthetic"`
	Graphs        []DemoGraph `json:"graphs"`
}

// DemoGraph stores each distinct ANSI line once; views reference lines by
// index and map key names to the view they lead to. An edge to -1 means the
// key leads somewhere the exploration limits left out, so the website can say
// that the step is not part of the demo instead of silently ignoring it.
type DemoGraph struct {
	ID    string          `json:"id"`
	Title string          `json:"title"`
	Start int             `json:"start"`
	Lines []string        `json:"lines"`
	Views []DemoGraphView `json:"views"`
}

type DemoGraphView struct {
	Lines []int          `json:"l"`
	Next  map[string]int `json:"k,omitempty"`
	// Text marks a focused text field; the demo does not accept typing.
	Text bool `json:"t,omitempty"`
	// Unexplored marks a view past the depth limit: its keys were not tried.
	Unexplored bool `json:"x,omitempty"`
}

// demoGraphTrimmed is the edge target for a step the limits left out.
const demoGraphTrimmed = -1

type demoGraphKey struct {
	name   string
	press  tea.KeyPressMsg
	letter bool
}

// demoProbeKey detects a focused text field: no dashboard shortcut uses it,
// so a changed view means the key was typed into an input.
var demoProbeKey = tea.KeyPressMsg{Code: 'ʒ', Text: "ʒ"}

func demoGraphKeys() []demoGraphKey {
	keys := []demoGraphKey{
		{name: "up", press: demoCode(tea.KeyUp)},
		{name: "down", press: demoCode(tea.KeyDown)},
		{name: "left", press: demoCode(tea.KeyLeft)},
		{name: "right", press: demoCode(tea.KeyRight)},
		{name: "enter", press: demoCode(tea.KeyEnter)},
		{name: "esc", press: demoCode(tea.KeyEscape)},
		{name: "tab", press: demoCode(tea.KeyTab)},
		{name: "space", press: demoCode(tea.KeySpace)},
		{name: "f1", press: demoCode(tea.KeyF1)},
	}
	// j and k repeat the arrow keys and q quits; they add no new screens.
	for _, letter := range "abcdefghilnprstuvwxy/" {
		keys = append(keys, demoGraphKey{name: string(letter), press: tea.KeyPressMsg{Code: letter, Text: string(letter)}, letter: true})
	}
	return keys
}

func RenderDemoGraphs(sourceCommit, sourceDate string) DemoGraphBundle {
	const width, height = 120, 30
	return DemoGraphBundle{
		SchemaVersion: 1,
		SourceCommit:  sourceCommit,
		SourceDate:    sourceDate,
		Terminal:      fmt.Sprintf("%dx%d", width, height),
		Synthetic:     true,
		Graphs: []DemoGraph{
			exploreDemoGraph("teacher", "Teacher dashboard", func() dashboardModel { return demoExploreModel(sourceCommit, width, height, true) }, demoGraphLimits{views: 900, perScreen: 160, depth: 16}),
			exploreDemoGraph("administrator", "Administrator menu", func() dashboardModel { return demoExploreModel(sourceCommit, width, height, false) }, demoGraphLimits{views: 2400, perScreen: 160, depth: 16}),
		},
	}
}

// demoObservedAt uses the local zone so the rendered clock reads 10:15 on any
// machine that regenerates the graphs.
var demoObservedAt = time.Date(2026, 9, 30, 10, 15, 0, 0, time.Local)

// demoExploreClients keeps the navigable lab small: selection lists grow
// with every subset of computers, and four clients still show each state.
const demoExploreClients = 4

// demoExploreHosts simulates a running lesson: three clients answer, pc02 has
// an active student session, pc03 is out of date and pc04 is switched off.
func demoExploreHosts(revision string) domain.HostsReport {
	hosts := domain.HostsReport{SchemaVersion: domain.SchemaVersion, Operation: "hosts", GeneratedAt: demoObservedAt, State: "ready", Repository: "/demo/lab", DesiredRevision: revision}
	for index := 1; index <= demoExploreClients; index++ {
		host := domain.HostStatus{Name: fmt.Sprintf("pc%02d", index), IP: fmt.Sprintf("10.42.0.%d", 10+index), Role: "client", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Deployment: domain.DeploymentCurrent, CurrentRevision: revision, DesiredRevision: revision}
		switch index {
		case 3:
			host.Deployment = domain.DeploymentOutdated
			host.CurrentRevision = strings.Repeat("0", 40)
		case 4:
			host.Reachability, host.SSH, host.Deployment, host.CurrentRevision = domain.ReachabilityUnreachable, "", domain.DeploymentUnknown, ""
		}
		hosts.Hosts = append(hosts.Hosts, host)
	}
	hosts.Deployment = domain.HostDeploymentSummary{Current: 2, Outdated: 1}
	return hosts
}

func demoExploreStatus(revision string) domain.StatusReport {
	status := demoStatus("ready", revision)
	status.Meta.Clients.Count = demoExploreClients
	status.Meta.Clients.Hosts = status.Meta.Clients.Hosts[:demoExploreClients]
	return status
}

func demoExploreTargets() []domain.DeploymentTarget {
	return demoDeploymentTargets()[:demoExploreClients]
}

func demoExploreClientNames() []string {
	names := make([]string, 0, demoExploreClients)
	for _, target := range demoExploreTargets() {
		names = append(names, target.Name)
	}
	return names
}

func demoExploreModel(revision string, width, height int, classroom bool) dashboardModel {
	actions := demoExploreActions(revision)
	actions.ClassroomMode = classroom
	model := newDashboardModel(demoExploreStatus(revision), demoSetupReady(), actions, false)
	model.width, model.height, model.isDark = width, height, true
	model.homeMenu = newDashboardTaskMenu(true, model.width, model.height)
	model.ensureActivitySpinner()
	model.computers.hosts = demoExploreHosts(revision)
	if classroom {
		model.screen = dashboardComputersArea
	}
	return model
}

// demoExploreActions answers reads and reviews with synthetic data. Actions
// left nil make the dashboard report the feature as unavailable, as it does
// for a real installation that lacks them.
func demoExploreActions(revision string) DashboardActions {
	hosts := demoExploreHosts(revision)
	catalog := demoSoftwareCatalog()
	catalog.Clients = demoExploreClientNames()
	catalog.Groups = map[string][]string{"graphics": {"pc01", "pc02"}}
	status := demoExploreStatus(revision)
	sessions := map[string]domain.ShutdownSessionState{"pc01": domain.ShutdownSessionIdle, "pc02": domain.ShutdownSessionActive, "pc03": domain.ShutdownSessionIdle}
	actions := DashboardActions{
		RunningVersion: "demo",
		LoadInventory:  func(context.Context) (domain.StatusReport, error) { return status, nil },
		Refresh:        func(context.Context) (domain.StatusReport, error) { return status, nil },
		LoadHosts:      func(context.Context) (domain.HostsReport, error) { return hosts, nil },
		LoadSetup:      func(context.Context) domain.SetupReport { return demoSetupReady() },
		LoadRecovery: func(context.Context) domain.RecoveryReport {
			return domain.RecoveryReport{SchemaVersion: domain.SchemaVersion, Operation: "recovery", GeneratedAt: demoObservedAt, State: "clear"}
		},
		PlanInternet: func(_ context.Context, requested string, action domain.InternetAction) domain.InternetPlan {
			plan := domain.InternetPlan{SchemaVersion: domain.SchemaVersion, Operation: "internet-plan", State: "ready", Repository: "/demo/lab", Requested: requested, Action: action, ReviewToken: "sha256:demo", Issues: []domain.ValidationIssue{}}
			for _, host := range hosts.Hosts {
				target := domain.InternetTarget{HostMeta: domain.HostMeta{Name: host.Name, IP: host.IP}}
				if host.Reachability == domain.ReachabilityReachable {
					target.Observed = domain.InternetObservation{SchemaVersion: domain.SchemaVersion, BootID: "12345678-1234-1234-1234-123456789abc", State: "enabled"}
					target.Eligible = true
				}
				if requested == "" || strings.Contains(","+requested+",", ","+host.Name+",") {
					plan.Targets = append(plan.Targets, target)
				}
			}
			return plan
		},
		PlanPower: func(_ context.Context, requested string, policy domain.ShutdownSessionPolicy, action domain.ClientPowerAction) domain.ShutdownPlanReport {
			confirmation := "SHUTDOWN"
			if action == domain.ClientRestart {
				confirmation = "RESTART"
			}
			plan := domain.ShutdownPlanReport{SchemaVersion: domain.SchemaVersion, Operation: "power-plan", State: "ready", Repository: "/demo/lab", Requested: requested, Action: action, Policy: policy, ReviewToken: "sha256:demo", Confirmation: confirmation, Issues: []domain.ValidationIssue{}}
			for _, host := range hosts.Hosts {
				if requested != "" && !strings.Contains(","+requested+",", ","+host.Name+",") {
					continue
				}
				target := domain.ShutdownTargetPlan{Name: host.Name, IP: host.IP, Reachability: host.Reachability, SSH: host.SSH, Session: sessions[host.Name], Eligible: host.Reachability == domain.ReachabilityReachable}
				if target.Eligible {
					plan.Eligible++
				}
				plan.Targets = append(plan.Targets, target)
			}
			return plan
		},
		LoadSoftware: func(context.Context) domain.SoftwareCatalogReport { return catalog },
		SearchSoftware: func(_ context.Context, query string) domain.SoftwareSearchReport {
			return domain.SoftwareSearchReport{SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-search", State: "ready", Repository: "/demo/lab", Query: query, Results: catalog.Catalog, Issues: []domain.ValidationIssue{}}
		},
		PlanSoftware: func(_ context.Context, request domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
			confirmation := "SAVE"
			if !request.Present {
				confirmation = "REMOVE"
			}
			return domain.SoftwareChangePlanReport{SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-change-plan", State: "ready", Repository: "/demo/lab", ManagedFile: "lab-software.json", Request: request, AffectedClients: demoExploreClientNames(), ReviewToken: "sha256:demo", Confirmation: confirmation, Issues: []domain.ValidationIssue{}}
		},
		SaveSoftware: func(plan domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport {
			return domain.SoftwareChangeApplyReport{SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-change-save", State: "saved", Repository: plan.Repository, ManagedFile: plan.ManagedFile, Request: plan.Request, AffectedClients: plan.AffectedClients, Revision: revision, Message: "Software selection saved locally.", Issues: []domain.ValidationIssue{}}
		},
		PlanDeployment: func(_ context.Context, requested string) domain.DeploymentPlanReport {
			plan := domain.DeploymentPlanReport{SchemaVersion: domain.SchemaVersion, Operation: "deploy-plan", State: "ready", Repository: "/demo/lab", Requested: requested, Revision: revision, ColmenaSelector: requested, BuildFirst: true, Issues: []domain.ValidationIssue{}}
			for _, host := range hosts.Hosts {
				if requested != "" && requested != "@lab" && !strings.Contains(","+requested+",", ","+host.Name+",") {
					continue
				}
				plan.Targets = append(plan.Targets, domain.DeploymentTarget{Name: host.Name, IP: host.IP})
				plan.Availability = append(plan.Availability, domain.DeploymentTargetAvailability{Name: host.Name, IP: host.IP, Reachability: host.Reachability, SSH: host.SSH})
			}
			return plan
		},
		LoadSettings:  func(context.Context) (domain.LabSettingsFile, error) { return demoSettings(), nil },
		LoadWorkspace: func(context.Context) domain.WorkspacePlanReport { return demoWorkspacePlan() },
		LoadLogs: func(context.Context) domain.OperationLogsReport {
			return domain.OperationLogsReport{SchemaVersion: domain.SchemaVersion, Operation: "logs", State: "ready", Limit: 20, Logs: []domain.OperationLogEntry{
				{ID: "deploy-lab", Kind: "deploy", StartedAt: demoObservedAt.Add(-26 * time.Hour), SizeBytes: 48213, State: "completed", Available: true},
				{ID: "controller-apply", Kind: "controller", StartedAt: demoObservedAt.Add(-27 * time.Hour), SizeBytes: 12840, State: "completed", Available: true},
			}, Records: []domain.OperationRecord{}, Issues: []domain.ValidationIssue{}}
		},
		CheckUpdate: func(context.Context) domain.UpdateCheckReport {
			return domain.UpdateCheckReport{SchemaVersion: domain.SchemaVersion, Operation: "update-check", GeneratedAt: demoObservedAt, State: "available", Repository: "/demo/lab", Upstream: "https://github.com/giovantenne/nixorium", CurrentRef: "v2.0.0", CurrentRev: revision, CurrentChannel: domain.UpdateChannelStable, Development: []domain.UpdateRelease{}, Stable: []domain.UpdateRelease{{Tag: "v2.0.0", ObjectID: revision, Channel: domain.UpdateChannelStable}}, Prerelease: []domain.UpdateRelease{}, Issues: []domain.ValidationIssue{}}
		},
		LoadLog: func(_ context.Context, id string) domain.OperationLogReport {
			entry := domain.OperationLogEntry{ID: id, Kind: "deploy", StartedAt: demoObservedAt.Add(-26 * time.Hour), SizeBytes: 48213, State: "completed", Available: true}
			return domain.OperationLogReport{SchemaVersion: domain.SchemaVersion, Operation: "log", State: "ready", Log: &entry, Content: "Validated clean revision\nBuilt 5 client systems\nActivated pc01 pc02 pc03 pc04 pc05 over SSH\nAll five clients report the reviewed revision.", Issues: []domain.ValidationIssue{}}
		},
		LoadServices: func(context.Context) domain.ServicesReport {
			return domain.ServicesReport{SchemaVersion: domain.SchemaVersion, Operation: "services", State: "ready", Repository: "/demo/lab", Issues: []domain.ValidationIssue{}, Services: []domain.ManagedService{
				{ID: "cache", Name: "Binary cache", Purpose: "Serves prepared systems to the clients", Mode: "always", State: "active", Healthy: true, Actions: []string{"restart"}, Units: []domain.ServiceState{{Name: "nixorium-harmonia.service", Loaded: true, Active: true, State: "active"}}},
				{ID: "pxe", Name: "Network boot", Purpose: "Installs computers from the network", Mode: "on demand", State: "inactive", Healthy: true, Actions: []string{}, Units: []domain.ServiceState{{Name: "nixorium-pxe.service", Loaded: true, State: "inactive"}}},
			}}
		},
		LoadRemoteInstall: func(context.Context) (domain.RemoteInstallResponse, error) {
			return domain.RemoteInstallResponse{State: "ready"}, nil
		},
		BackupDestination: func() string { return "/run/media/admin/USB" },
	}
	reset := demoTemplateResetPlan()
	actions.LoadTemplateReset = func(context.Context) domain.TemplateResetCatalog {
		return domain.TemplateResetCatalog{UpstreamRevision: reset.UpstreamRevision, Catalog: domain.SoftwarePresetCatalog{Presets: []domain.SoftwarePreset{reset.Preset}}}
	}
	actions.PlanTemplateReset = func(context.Context, string, func(string)) domain.TemplateResetPlan { return reset }
	demoStubMissingActions(&actions)
	return actions
}

// demoStubMissingActions gives every remaining callback an empty answer. Some
// reads run in their own goroutine and do not check for a nil action, so a
// missing callback would otherwise crash the explorer instead of one edge.
func demoStubMissingActions(actions *DashboardActions) {
	value := reflect.ValueOf(actions).Elem()
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if field.Kind() != reflect.Func || !field.IsNil() {
			continue
		}
		kind := field.Type()
		field.Set(reflect.MakeFunc(kind, func([]reflect.Value) []reflect.Value {
			results := make([]reflect.Value, kind.NumOut())
			for position := range results {
				results[position] = reflect.Zero(kind.Out(position))
			}
			return results
		}))
	}
}

// demoGraphLimits bounds the exploration. Selection lists multiply states
// (cursor position times every subset of computers), so each screen keeps
// only its views closest to the start; breadth-first order makes those the
// natural ones.
type demoGraphLimits struct {
	views     int
	perScreen int
	depth     int
}

type demoExplorer struct {
	start    func() dashboardModel
	keys     []demoGraphKey
	limits   demoGraphLimits
	graph    DemoGraph
	byView   map[string]int
	byLine   map[string]int
	byScreen map[string]int
	paths    [][]int
	views    []string
}

func exploreDemoGraph(id, title string, start func() dashboardModel, limits demoGraphLimits) DemoGraph {
	explorer := demoExplorer{start: start, keys: demoGraphKeys(), limits: limits, graph: DemoGraph{ID: id, Title: title}, byView: map[string]int{}, byLine: map[string]int{}, byScreen: map[string]int{}}
	root, ok := explorer.replay(nil)
	if !ok {
		panic("demo graph root could not be rendered: " + id)
	}
	explorer.add(demoGraphView(root), nil)
	// Views waiting in the queue already know their paths, so a window of them
	// is explored in parallel; merging in queue order keeps the result equal
	// to a sequential breadth-first exploration.
	window := 4 * runtime.GOMAXPROCS(0)
	for next := 0; next < len(explorer.paths); {
		end := min(next+window, len(explorer.paths))
		explored := explorer.exploreWindow(explorer.paths[next:end])
		for offset, result := range explored {
			explorer.merge(next+offset, result)
		}
		next = end
	}
	return explorer.graph
}

type demoGraphCandidate struct {
	path []int
	view string
	ok   bool
}

type demoGraphExplored struct {
	unexplored bool
	typing     bool
	candidates []demoGraphCandidate
}

// exploreWindow replays the keys of several queued views in parallel. Each
// replay starts from a fresh model, so the work shares no dashboard state.
func (e *demoExplorer) exploreWindow(paths [][]int) []demoGraphExplored {
	results := make([]demoGraphExplored, len(paths))
	var wait sync.WaitGroup
	for index, path := range paths {
		if len(path) >= e.limits.depth {
			results[index].unexplored = true
			continue
		}
		wait.Add(1)
		go func(index int, path []int) {
			defer wait.Done()
			typing := e.acceptsText(path)
			candidates := make([]demoGraphCandidate, len(e.keys))
			for key, item := range e.keys {
				if item.letter && typing {
					continue
				}
				candidate := append(append([]int{}, path...), key)
				if model, ok := e.replay(candidate); ok {
					candidates[key] = demoGraphCandidate{path: candidate, view: demoGraphView(model), ok: true}
				}
			}
			results[index] = demoGraphExplored{typing: typing, candidates: candidates}
		}(index, path)
	}
	wait.Wait()
	return results
}

// merge records one explored view in queue order, adding new views and
// marking the steps that the limits leave out.
func (e *demoExplorer) merge(id int, result demoGraphExplored) {
	if result.unexplored {
		e.graph.Views[id].Unexplored = true
		return
	}
	e.graph.Views[id].Text = result.typing
	for index, candidate := range result.candidates {
		if !candidate.ok || candidate.view == e.views[id] {
			continue
		}
		key := e.keys[index]
		target, seen := e.byView[candidate.view]
		if !seen {
			// Esc must always lead back, or a capped screen becomes a dead end.
			if len(e.views) >= e.limits.views || (key.name != "esc" && e.byScreen[demoGraphScreen(candidate.view)] >= e.limits.perScreen) {
				target = demoGraphTrimmed
			} else {
				target = e.add(candidate.view, candidate.path)
			}
		}
		if e.graph.Views[id].Next == nil {
			e.graph.Views[id].Next = map[string]int{}
		}
		e.graph.Views[id].Next[key.name] = target
	}
}

func (e *demoExplorer) add(view string, path []int) int {
	id := len(e.views)
	e.byView[view] = id
	e.byScreen[demoGraphScreen(view)]++
	e.views = append(e.views, view)
	e.paths = append(e.paths, path)
	lines := strings.Split(view, "\n")
	indexes := make([]int, len(lines))
	for position, line := range lines {
		index, seen := e.byLine[line]
		if !seen {
			index = len(e.graph.Lines)
			e.byLine[line] = index
			e.graph.Lines = append(e.graph.Lines, line)
		}
		indexes[position] = index
	}
	e.graph.Views = append(e.graph.Views, DemoGraphView{Lines: indexes})
	return id
}

func (e *demoExplorer) acceptsText(path []int) bool {
	model, ok := e.replay(path)
	if !ok {
		return true
	}
	before := demoGraphView(model)
	probed, ok := demoSettle(model, demoUpdate(&model, demoProbeKey))
	return !ok || demoGraphView(probed) != before
}

// replay rebuilds a state from the root. Models share maps and slices
// between copies, so branching from a stored copy could leak selections into
// sibling states; replaying keeps every edge independent.
func (e *demoExplorer) replay(path []int) (model dashboardModel, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	model = e.start()
	for _, index := range path {
		command := demoUpdate(&model, e.keys[index].press)
		if model, ok = demoSettle(model, command); !ok {
			return model, false
		}
	}
	return model, true
}

func demoUpdate(model *dashboardModel, message tea.Msg) tea.Cmd {
	updated, command := model.Update(message)
	*model = updated.(dashboardModel)
	return command
}

// demoSettle runs follow-up commands to completion. Timer, spinner and
// cursor-blink ticks are skipped: they only animate or poll, and waiting for
// them would make the result depend on the clock. Paths are replayed from the
// start, so one waited blink would be paid again on every replay through a
// text field.
func demoSettle(model dashboardModel, command tea.Cmd) (dashboardModel, bool) {
	pending := []tea.Cmd{command}
	for steps := 0; len(pending) > 0 && steps < 64; steps++ {
		next := pending[0]
		pending = pending[1:]
		if next == nil || demoTimerCommand(next) {
			continue
		}
		message, ok := demoRunCommand(next)
		if !ok {
			return model, false
		}
		switch value := message.(type) {
		case nil, tea.QuitMsg:
			continue
		case tea.BatchMsg:
			pending = append(pending, value...)
			continue
		}
		if commands, isList := demoCommandList(message); isList {
			pending = append(pending, commands...)
			continue
		}
		pending = append(pending, demoUpdate(&model, message))
	}
	return model, true
}

func demoCommandList(message tea.Msg) ([]tea.Cmd, bool) {
	value := reflect.ValueOf(message)
	if value.Kind() != reflect.Slice || value.Type().Elem() != reflect.TypeOf(tea.Cmd(nil)) {
		return nil, false
	}
	commands := make([]tea.Cmd, value.Len())
	for index := range commands {
		commands[index] = value.Index(index).Interface().(tea.Cmd)
	}
	return commands, true
}

func demoTimerCommand(command tea.Cmd) bool {
	function := runtime.FuncForPC(reflect.ValueOf(command).Pointer())
	if function == nil {
		return false
	}
	name := function.Name()
	return strings.Contains(name, "bubbletea/v2.Tick.") || strings.Contains(name, "bubbletea/v2.Every.") || strings.Contains(name, "bubbles/v2/spinner.") || strings.Contains(name, "bubbles/v2/cursor.")
}

// demoRunCommand runs one command, recovering a panic in a callback. The
// time limit only guards against a command that never returns.
func demoRunCommand(command tea.Cmd) (tea.Msg, bool) {
	type result struct {
		message tea.Msg
		ok      bool
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if recover() != nil {
				done <- result{ok: false}
			}
		}()
		done <- result{message: command(), ok: true}
	}()
	select {
	case outcome := <-done:
		return outcome.message, outcome.ok
	case <-time.After(10 * time.Second):
		panic("demo command did not finish: " + runtime.FuncForPC(reflect.ValueOf(command).Pointer()).Name())
	}
}

var demoGraphDigits = regexp.MustCompile(`[0-9]+`)

// demoGraphScreen names a view by its breadcrumb and heading, so a review
// keeps its own budget instead of sharing the selection list's. Counts are
// ignored: "Shut down 2 clients?" is the same screen as "Shut down 3".
func demoGraphScreen(view string) string {
	var parts []string
	for _, line := range strings.Split(demoGraphDigits.ReplaceAllString(demoANSI.ReplaceAllString(view, ""), "#"), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			if parts = append(parts, trimmed); len(parts) == 2 {
				break
			}
		}
	}
	return strings.Join(parts, "\n")
}

func demoGraphView(model dashboardModel) string {
	model.busyStarted = time.Time{}
	model.controller.started = time.Time{}
	model.deployment.started = time.Time{}
	model.installation.pxeStarted = time.Time{}
	view := strings.TrimRight(model.View().Content, " \n")
	return demoGraphClock.ReplaceAllString(view, demoObservedAt.Format("15:04:05"))
}

// demoGraphClock matches times the dashboard stamps with the current clock,
// such as an Internet check; the graph shows the fixture's time instead.
var demoGraphClock = regexp.MustCompile(`\b(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]\b`)
