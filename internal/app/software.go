package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

type SoftwareSource interface {
	SoftwareDefinition(context.Context, string) (domain.SoftwareDefinition, error)
	SearchSoftwarePackages(context.Context, string, string, int) ([]domain.SoftwareCatalogItem, error)
	ResolveSoftwarePackage(context.Context, string, string) (domain.SoftwareCatalogItem, error)
	ReadSoftware(string) ([]byte, error)
	ValidateSoftwareCandidate(context.Context, string, domain.LabSoftwareFile) error
	WriteSoftwareIfUnchanged(string, []byte, domain.LabSoftwareFile) error
}

type SoftwareManager struct{ source SoftwareSource }

func NewSoftwareManager(source SoftwareSource) SoftwareManager {
	return SoftwareManager{source: source}
}

func (m SoftwareManager) Search(ctx context.Context, repository, query string) domain.SoftwareSearchReport {
	report := domain.SoftwareSearchReport{
		SchemaVersion: domain.SoftwareSchemaVersion,
		Operation:     "software-search",
		State:         "invalid",
		Repository:    repository,
		Query:         query,
		Results:       []domain.SoftwareCatalogItem{},
		Issues:        []domain.ValidationIssue{},
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return softwareSearchIssue(report, "repository", err.Error())
	}
	report.Repository = root
	query = strings.TrimSpace(query)
	report.Query = query
	if err := domain.ValidateSoftwareSearchQuery(query); err != nil {
		return softwareSearchIssue(report, "query", err.Error())
	}
	items, err := m.source.SearchSoftwarePackages(ctx, root, query, 40)
	if err != nil {
		return softwareSearchIssue(report, "search", err.Error())
	}
	for _, item := range items {
		if err := validateSoftwarePackageItem(item); err != nil {
			return softwareSearchIssue(report, "results", err.Error())
		}
		report.Results = append(report.Results, item)
	}
	report.State = "ready"
	if len(report.Results) == 0 {
		report.Message = "No packages in the deployment's pinned package set match this name."
	} else {
		report.Message = fmt.Sprintf("Found %d package(s) in the deployment's pinned package set.", len(report.Results))
	}
	return report
}

func (m SoftwareManager) Catalog(ctx context.Context, repository string) domain.SoftwareCatalogReport {
	root, err := filepath.Abs(repository)
	if err != nil {
		return softwareCatalogFailure(repository, "repository", err.Error())
	}
	definition, err := m.source.SoftwareDefinition(ctx, root)
	if err != nil {
		return softwareCatalogFailure(root, "catalog", err.Error())
	}
	if err := validateSoftwareDefinition(definition); err != nil {
		return softwareCatalogFailure(root, "catalog", err.Error())
	}
	return domain.SoftwareCatalogReport{
		SchemaVersion: domain.SoftwareSchemaVersion,
		Operation:     "software-catalog",
		State:         "ready",
		Repository:    root,
		ManagedFile:   definition.ManagedFile,
		Controller:    definition.Controller,
		Clients:       append([]string(nil), definition.Clients...),
		Groups:        cloneSoftwareGroups(definition.Groups),
		Catalog:       append([]domain.SoftwareCatalogItem(nil), definition.Catalog...),
		Packages:      append([]domain.SoftwareDeclaration(nil), definition.Packages...),
		Issues:        []domain.ValidationIssue{},
		Message:       "Supported software was resolved from the pinned package set.",
	}
}

func (m SoftwareManager) Plan(ctx context.Context, repository string, request domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
	catalog := m.Catalog(ctx, repository)
	report := domain.SoftwareChangePlanReport{
		SchemaVersion:   domain.SoftwareSchemaVersion,
		Operation:       "software-change-plan",
		State:           "invalid",
		Repository:      catalog.Repository,
		ManagedFile:     catalog.ManagedFile,
		Request:         request,
		Candidate:       domain.LabSoftwareFile{SchemaVersion: domain.SoftwareSchemaVersion, Packages: []domain.SoftwareDeclaration{}},
		AffectedClients: []string{},
		Issues:          append([]domain.ValidationIssue(nil), catalog.Issues...),
	}
	if catalog.HasErrors() {
		report.Message = catalog.Message
		return report
	}
	if !softwareIDValid(request.Package) {
		return softwarePlanIssue(report, "package", "select a valid package attribute from the pinned package search")
	}
	baseData, err := m.source.ReadSoftware(catalog.Repository)
	if err != nil {
		return softwarePlanIssue(report, "file", err.Error())
	}
	base, err := domain.DecodeLabSoftware(baseData)
	if err != nil {
		return softwarePlanIssue(report, "file", err.Error())
	}
	report.BaseFingerprint = domain.SoftwareFingerprint(baseData)
	request.Scope = normalizeSoftwareScope(request.Scope)
	existing, exists := softwareDeclaration(base.Packages, request.Package)
	item := domain.SoftwareCatalogItem{ID: request.Package, Label: request.Package, Summary: "Configured package", Availability: "available"}
	if request.Present {
		item, err = m.source.ResolveSoftwarePackage(ctx, catalog.Repository, request.Package)
		if err != nil {
			return softwarePlanIssue(report, "package", "resolve package from pinned inputs: "+err.Error())
		}
		if err := validateSoftwarePackageItem(item); err != nil {
			return softwarePlanIssue(report, "package", err.Error())
		}
		if item.Availability != "available" {
			return softwarePlanIssue(report, "package", "the selected package is "+item.Availability+" in this deployment")
		}
	} else if !exists {
		report.State = "unchanged"
		report.Message = request.Package + " is not present in the managed software configuration."
		return report
	}
	if !request.Present && exists {
		request.Scope = existing.Scope
	}
	if err := validateSoftwareRequestScope(request.Scope, catalog.Clients, catalog.Groups); err != nil {
		return softwarePlanIssue(report, "scope", err.Error())
	}
	if softwareScopeIncludesController(request.Scope) && catalog.Controller == "" {
		return softwarePlanIssue(report, "scope", "this deployment does not support controller software; update Nixorium before using this scope")
	}
	report.Request = request
	report.AffectedClients = softwareScopeClients(request.Scope, catalog.Clients, catalog.Groups)
	if softwareScopeIncludesController(request.Scope) {
		report.AffectedController = catalog.Controller
	}
	previousClients := []string{}
	// Changing scope can remove software from old targets as well as add it to
	// new ones. Review the union, not just the requested destinations.
	if exists {
		previousClients = softwareScopeClients(existing.Scope, catalog.Clients, catalog.Groups)
		for _, name := range previousClients {
			if !containsSoftwareClient(report.AffectedClients, name) {
				report.AffectedClients = append(report.AffectedClients, name)
			}
		}
		sort.Strings(report.AffectedClients)
		if softwareScopeIncludesController(existing.Scope) {
			report.AffectedController = catalog.Controller
		}
	}
	candidate := domain.NormalizeLabSoftware(changeSoftwareDeclaration(base, request))
	report.Candidate = candidate
	candidateData, err := domain.MarshalLabSoftware(candidate)
	if err != nil {
		return softwarePlanIssue(report, "candidate", err.Error())
	}
	baseNormalized, err := domain.MarshalLabSoftware(base)
	if err != nil {
		return softwarePlanIssue(report, "file", err.Error())
	}
	if string(candidateData) == string(baseNormalized) {
		report.State = "unchanged"
		report.Message = item.Label + " already has the requested declaration."
		return report
	}
	if err := m.source.ValidateSoftwareCandidate(ctx, catalog.Repository, candidate); err != nil {
		return softwarePlanIssue(report, "validation", "Nix evaluation rejected the software proposal: "+err.Error())
	}
	report.State = "ready"
	// Bind review to the evaluated destinations too: a group/inventory change
	// can alter the impact even when the declaration bytes are identical.
	reviewData, err := json.Marshal(struct {
		Candidate        json.RawMessage `json:"candidate"`
		Controller       string          `json:"controller"`
		PreviousClients  []string        `json:"previousClients"`
		RequestedClients []string        `json:"requestedClients"`
	}{candidateData, report.AffectedController, previousClients, softwareScopeClients(request.Scope, catalog.Clients, catalog.Groups)})
	if err != nil {
		return softwarePlanIssue(report, "review", err.Error())
	}
	report.ReviewToken = domain.SoftwareReviewToken(report.BaseFingerprint, reviewData)
	report.Confirmation = "SAVE"
	action := "Add "
	if !request.Present {
		report.Confirmation = "REMOVE"
		action = "Remove "
	}
	report.Message = action + item.Label + " in " + report.ManagedFile + "; no system has been built or changed."
	return report
}

func (m SoftwareManager) ApplyPlan(ctx context.Context, plan domain.SoftwareChangePlanReport, expectedToken string) domain.SoftwareChangeApplyReport {
	report := domain.SoftwareChangeApplyReport{
		SchemaVersion:      domain.SoftwareSchemaVersion,
		Operation:          "software-change-apply",
		State:              "invalid",
		Repository:         plan.Repository,
		ManagedFile:        plan.ManagedFile,
		Request:            plan.Request,
		AffectedController: plan.AffectedController,
		AffectedClients:    append([]string(nil), plan.AffectedClients...),
		Issues:             []domain.ValidationIssue{},
	}
	fresh := m.Plan(ctx, plan.Repository, plan.Request)
	if fresh.HasErrors() {
		report.Issues = append(report.Issues, fresh.Issues...)
		report.Message = fresh.Message
		return report
	}
	if fresh.State == "unchanged" {
		report.State = "unchanged"
		report.Message = fresh.Message
		return report
	}
	if expectedToken == "" || expectedToken != plan.ReviewToken || expectedToken != fresh.ReviewToken || plan.ManagedFile != fresh.ManagedFile {
		report.State = "conflict"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "reviewToken", Message: "the software proposal changed after review; create a new plan"})
		report.Message = "Software was not changed because the reviewed proposal is stale."
		return report
	}
	baseData, err := m.source.ReadSoftware(fresh.Repository)
	if err != nil || domain.SoftwareFingerprint(baseData) != fresh.BaseFingerprint {
		report.State = "conflict"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "file", Message: "lab-software.json changed while applying; create a new plan"})
		report.Message = "Software was not changed because the managed file changed."
		return report
	}
	if err := m.source.WriteSoftwareIfUnchanged(fresh.Repository, baseData, fresh.Candidate); err != nil {
		if errors.Is(err, domain.ErrSoftwareConflict) {
			report.State = "conflict"
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: "file", Message: "lab-software.json changed while applying; create a new plan"})
			report.Message = "Software was not changed because the managed file changed."
		} else if errors.Is(err, domain.ErrSoftwareDurability) {
			report.State = "partial"
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: "durability", Message: err.Error()})
			report.Message = "lab-software.json was replaced, but durable storage could not be confirmed. Inspect the file and Git state before creating another proposal."
		} else {
			report.State = "failed"
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: "write", Message: err.Error()})
			report.Message = "Software declaration was not saved."
		}
		return report
	}
	report.State = "applied"
	report.Repository = fresh.Repository
	report.ManagedFile = fresh.ManagedFile
	report.Request = fresh.Request
	report.AffectedController = fresh.AffectedController
	report.AffectedClients = append([]string(nil), fresh.AffectedClients...)
	report.Message = "Software declaration saved. Review and commit lab-software.json before preparing or distributing systems."
	return report
}

func softwareCatalogFailure(repository, field, message string) domain.SoftwareCatalogReport {
	return domain.SoftwareCatalogReport{SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-catalog", State: "failed", Repository: repository, Clients: []string{}, Groups: map[string][]string{}, Catalog: []domain.SoftwareCatalogItem{}, Packages: []domain.SoftwareDeclaration{}, Issues: []domain.ValidationIssue{{Field: field, Message: message}}, Message: message}
}

func softwarePlanIssue(report domain.SoftwareChangePlanReport, field, message string) domain.SoftwareChangePlanReport {
	report.State = "invalid"
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	report.Message = message
	return report
}

func softwareSearchIssue(report domain.SoftwareSearchReport, field, message string) domain.SoftwareSearchReport {
	report.State = "invalid"
	if field != "query" {
		report.State = "failed"
	}
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	report.Message = message
	return report
}

func validateSoftwarePackageItem(item domain.SoftwareCatalogItem) error {
	if !softwareIDValid(item.ID) || item.Label == "" || item.Summary == "" {
		return errors.New("package search returned an invalid item")
	}
	switch item.Availability {
	case "available", "blocked-broken", "blocked-insecure", "blocked-unfree", "unavailable-platform":
		return nil
	default:
		return errors.New("package search returned an unknown availability state")
	}
}

func validateSoftwareDefinition(definition domain.SoftwareDefinition) error {
	if definition.SchemaVersion != domain.SoftwareSchemaVersion || definition.ManagedFile != "lab-software.json" {
		return errors.New("deployment exposes an unsupported software contract")
	}
	clients := map[string]bool{}
	for _, name := range definition.Clients {
		if !isClientName(name) || clients[name] {
			return errors.New("software contract contains an invalid client inventory")
		}
		clients[name] = true
	}
	if definition.Controller != "" && (!isClientName(definition.Controller) || clients[definition.Controller]) {
		return errors.New("software contract contains an invalid controller identity")
	}
	for group, names := range definition.Groups {
		if err := domain.ValidateSoftwareScope(domain.SoftwareScope{Kind: domain.SoftwareScopeGroup, Group: group}); err != nil || len(names) == 0 {
			return errors.New("software contract contains an invalid client group")
		}
		groupClients := map[string]bool{}
		for _, name := range names {
			if !clients[name] || groupClients[name] {
				return errors.New("software contract group contains an unknown or duplicate client")
			}
			groupClients[name] = true
		}
	}
	seen := map[string]bool{}
	for _, item := range definition.Catalog {
		if validateSoftwarePackageItem(item) != nil || item.Availability != "available" || seen[item.ID] {
			return errors.New("software contract contains an invalid catalog item")
		}
		seen[item.ID] = true
	}
	managed := map[string]bool{}
	for _, entry := range definition.Packages {
		if entry.Origin != "managed" || managed[entry.Package] {
			return errors.New("software contract contains an unsupported managed package")
		}
		managed[entry.Package] = true
		entry.Origin = ""
		if softwareScopeIncludesController(entry.Scope) && definition.Controller == "" {
			return errors.New("software contract does not advertise controller software support")
		}
		if err := domain.ValidateLabSoftware(domain.LabSoftwareFile{SchemaVersion: domain.SoftwareSchemaVersion, Packages: []domain.SoftwareDeclaration{entry}}); err != nil {
			return errors.New("software contract contains an invalid managed declaration")
		}
		if err := validateSoftwareRequestScope(entry.Scope, definition.Clients, definition.Groups); err != nil {
			return errors.New("software contract contains a declaration outside the evaluated inventory")
		}
	}
	return nil
}

func softwareIDValid(value string) bool {
	_, err := domain.MarshalLabSoftware(domain.LabSoftwareFile{SchemaVersion: domain.SoftwareSchemaVersion, Packages: []domain.SoftwareDeclaration{{Package: value, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}}}})
	return err == nil
}

func isClientName(value string) bool {
	return len(value) >= 3 && value[:2] == "pc" && softwareDigits(value[2:])
}

func softwareDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func softwareDeclaration(entries []domain.SoftwareDeclaration, id string) (domain.SoftwareDeclaration, bool) {
	for _, entry := range entries {
		if entry.Package == id {
			return entry, true
		}
	}
	return domain.SoftwareDeclaration{}, false
}

func normalizeSoftwareScope(scope domain.SoftwareScope) domain.SoftwareScope {
	scope.Clients = append([]string(nil), scope.Clients...)
	sort.Strings(scope.Clients)
	return scope
}

func validateSoftwareRequestScope(scope domain.SoftwareScope, clients []string, groups map[string][]string) error {
	if err := domain.ValidateSoftwareScope(scope); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, name := range clients {
		known[name] = true
	}
	switch scope.Kind {
	case domain.SoftwareScopeGroup:
		if _, exists := groups[scope.Group]; !exists {
			return fmt.Errorf("group %q is not in the evaluated deployment", scope.Group)
		}
	case domain.SoftwareScopeClients:
		for _, name := range scope.Clients {
			if !known[name] {
				return fmt.Errorf("client %q is not in the evaluated inventory", name)
			}
		}
	}
	return nil
}

func softwareScopeClients(scope domain.SoftwareScope, clients []string, groups map[string][]string) []string {
	var result []string
	switch scope.Kind {
	case domain.SoftwareScopeShared, domain.SoftwareScopeAllClients:
		result = append(result, clients...)
	case domain.SoftwareScopeGroup:
		result = append(result, groups[scope.Group]...)
	case domain.SoftwareScopeClients:
		result = append(result, scope.Clients...)
	}
	sort.Strings(result)
	return result
}

func changeSoftwareDeclaration(base domain.LabSoftwareFile, request domain.SoftwareChangeRequest) domain.LabSoftwareFile {
	result := domain.LabSoftwareFile{SchemaVersion: base.SchemaVersion, Packages: []domain.SoftwareDeclaration{}}
	for _, entry := range base.Packages {
		if entry.Package != request.Package {
			result.Packages = append(result.Packages, entry)
		}
	}
	if request.Present {
		result.Packages = append(result.Packages, domain.SoftwareDeclaration{Package: request.Package, Scope: request.Scope})
	}
	return result
}

func softwareScopeIncludesController(scope domain.SoftwareScope) bool {
	return scope.Kind == domain.SoftwareScopeShared || scope.Kind == domain.SoftwareScopeController
}

func containsSoftwareClient(clients []string, name string) bool {
	for _, client := range clients {
		if client == name {
			return true
		}
	}
	return false
}

func cloneSoftwareGroups(groups map[string][]string) map[string][]string {
	result := make(map[string][]string, len(groups))
	for name, clients := range groups {
		result[name] = append([]string(nil), clients...)
	}
	return result
}
