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

func (m SoftwareManager) Presets(ctx context.Context, repository string) domain.SoftwarePresetCatalogReport {
	report := domain.SoftwarePresetCatalogReport{
		SchemaVersion: domain.SoftwarePresetSchemaVersion,
		Operation:     "software-presets",
		State:         "failed",
		Repository:    repository,
		Issues:        []domain.ValidationIssue{},
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return softwarePresetCatalogIssue(report, "repository", err.Error())
	}
	report.Repository = root
	catalog, err := m.source.SoftwarePresetCatalog(ctx, root)
	if err != nil {
		return softwarePresetCatalogIssue(report, "catalog", err.Error())
	}
	if catalog == nil {
		report.State = "unavailable"
		report.Message = "This deployment does not provide software profiles. Individual software management remains available."
		return report
	}
	normalized := domain.NormalizeSoftwarePresetCatalog(*catalog)
	if err := domain.ValidateSoftwarePresetCatalog(normalized); err != nil {
		return softwarePresetCatalogIssue(report, "catalog", err.Error())
	}
	fingerprint, err := domain.SoftwarePresetCatalogFingerprint(normalized)
	if err != nil {
		return softwarePresetCatalogIssue(report, "catalog", err.Error())
	}
	report.State = "ready"
	report.Catalog = &normalized
	report.Fingerprint = fingerprint
	report.Message = fmt.Sprintf("%d deployment-owned software profile(s) available.", len(normalized.Presets))
	return report
}

func (m SoftwareManager) PlanPreset(ctx context.Context, repository string, request domain.SoftwarePresetRequest) domain.SoftwarePresetPlanReport {
	software := m.Catalog(ctx, repository)
	presets := m.Presets(ctx, repository)
	report := domain.SoftwarePresetPlanReport{
		SchemaVersion:      domain.SoftwarePresetSchemaVersion,
		Operation:          "software-preset-plan",
		State:              "invalid",
		Repository:         software.Repository,
		ManagedFile:        software.ManagedFile,
		Request:            request,
		SelectedPackages:   []domain.SoftwareCatalogItem{},
		Existing:           []domain.SoftwareDeclaration{},
		Additions:          []domain.SoftwareDeclaration{},
		Candidate:          domain.LabSoftwareFile{SchemaVersion: domain.SoftwareSchemaVersion, Packages: []domain.SoftwareDeclaration{}},
		AffectedClients:    []string{},
		Issues:             []domain.ValidationIssue{},
		CatalogFingerprint: presets.Fingerprint,
	}
	if software.HasErrors() {
		report.Issues = append(report.Issues, software.Issues...)
		report.Message = software.Message
		return report
	}
	if presets.State == "unavailable" {
		return softwarePresetPlanIssue(report, "catalog", presets.Message)
	}
	if presets.HasErrors() || presets.Catalog == nil {
		report.Issues = append(report.Issues, presets.Issues...)
		report.Message = presets.Message
		return report
	}
	request.Preset = strings.TrimSpace(request.Preset)
	request.Scope = normalizeSoftwareScope(request.Scope)
	request.Exclude = normalizeSoftwarePresetExclusions(request.Exclude)
	report.Request = request
	preset, found := softwarePresetByID(*presets.Catalog, request.Preset)
	if !found {
		return softwarePresetPlanIssue(report, "preset", fmt.Sprintf("software profile %q is not in the deployment catalog", request.Preset))
	}
	report.Preset = preset
	if err := validateSoftwareRequestScope(request.Scope, software.Clients, software.Groups); err != nil {
		return softwarePresetPlanIssue(report, "scope", err.Error())
	}
	if softwareScopeIncludesController(request.Scope) && software.Controller == "" {
		return softwarePresetPlanIssue(report, "scope", "this deployment does not support controller software; update Nixorium before using this scope")
	}
	presetPackages := map[string]bool{}
	for _, packageID := range preset.Packages {
		presetPackages[packageID] = true
	}
	for _, packageID := range request.Exclude {
		if !presetPackages[packageID] {
			return softwarePresetPlanIssue(report, "exclude", fmt.Sprintf("package %q is not part of software profile %s", packageID, preset.ID))
		}
	}
	excluded := map[string]bool{}
	for _, packageID := range request.Exclude {
		excluded[packageID] = true
	}

	baseData, err := m.source.ReadSoftware(software.Repository)
	if err != nil {
		return softwarePresetPlanIssue(report, "file", err.Error())
	}
	base, err := domain.DecodeLabSoftware(baseData)
	if err != nil {
		return softwarePresetPlanIssue(report, "file", err.Error())
	}
	report.BaseFingerprint = domain.SoftwareFingerprint(baseData)
	for _, packageID := range preset.Packages {
		if excluded[packageID] {
			continue
		}
		item, resolveErr := m.source.ResolveSoftwarePackage(ctx, software.Repository, packageID)
		if resolveErr != nil {
			return softwarePresetPlanIssue(report, "package", fmt.Sprintf("resolve %s from pinned inputs: %v", packageID, resolveErr))
		}
		if err := validateSoftwarePackageItem(item); err != nil {
			return softwarePresetPlanIssue(report, "package", err.Error())
		}
		if item.Availability != "available" {
			return softwarePresetPlanIssue(report, "package", fmt.Sprintf("package %s is %s in this deployment", packageID, item.Availability))
		}
		report.SelectedPackages = append(report.SelectedPackages, item)
		if existing, exists := softwareDeclaration(base.Packages, packageID); exists {
			report.Existing = append(report.Existing, existing)
			continue
		}
		report.Additions = append(report.Additions, domain.SoftwareDeclaration{Package: packageID, Scope: request.Scope})
	}

	candidate := domain.LabSoftwareFile{SchemaVersion: base.SchemaVersion, Packages: append([]domain.SoftwareDeclaration(nil), base.Packages...)}
	candidate.Packages = append(candidate.Packages, report.Additions...)
	candidate = domain.NormalizeLabSoftware(candidate)
	report.Candidate = candidate
	candidateData, err := domain.MarshalLabSoftware(candidate)
	if err != nil {
		return softwarePresetPlanIssue(report, "candidate", err.Error())
	}
	baseNormalized, err := domain.MarshalLabSoftware(base)
	if err != nil {
		return softwarePresetPlanIssue(report, "file", err.Error())
	}
	if string(candidateData) == string(baseNormalized) {
		report.State = "unchanged"
		report.Message = "Every selected package is already declared; existing scopes were preserved."
		return report
	}
	if err := m.source.ValidateSoftwareCandidate(ctx, software.Repository, candidate); err != nil {
		return softwarePresetPlanIssue(report, "validation", "Nix evaluation rejected the software profile proposal: "+err.Error())
	}
	report.AffectedClients = softwareScopeClients(request.Scope, software.Clients, software.Groups)
	if softwareScopeIncludesController(request.Scope) {
		report.AffectedController = software.Controller
	}
	reviewData, err := json.Marshal(struct {
		CatalogFingerprint string                       `json:"catalogFingerprint"`
		Request            domain.SoftwarePresetRequest `json:"request"`
		Selected           []domain.SoftwareCatalogItem `json:"selected"`
		Candidate          json.RawMessage              `json:"candidate"`
		Controller         string                       `json:"controller"`
		Clients            []string                     `json:"clients"`
	}{report.CatalogFingerprint, report.Request, report.SelectedPackages, candidateData, report.AffectedController, report.AffectedClients})
	if err != nil {
		return softwarePresetPlanIssue(report, "review", err.Error())
	}
	report.ReviewToken = domain.SoftwareReviewToken(report.BaseFingerprint, reviewData)
	report.Confirmation = "ADD PROFILE"
	report.State = "ready"
	report.Message = fmt.Sprintf("Add %d package declaration(s) from %s; %d existing declaration(s) keep their current scope. No system has been built or changed.", len(report.Additions), preset.Label, len(report.Existing))
	return report
}

func (m SoftwareManager) ApplyPresetPlan(ctx context.Context, plan domain.SoftwarePresetPlanReport, expectedToken string) domain.SoftwarePresetApplyReport {
	report := domain.SoftwarePresetApplyReport{
		SchemaVersion:      domain.SoftwarePresetSchemaVersion,
		Operation:          "software-preset-apply",
		State:              "invalid",
		Repository:         plan.Repository,
		ManagedFile:        plan.ManagedFile,
		Request:            plan.Request,
		Preset:             plan.Preset,
		Existing:           append([]domain.SoftwareDeclaration(nil), plan.Existing...),
		Additions:          append([]domain.SoftwareDeclaration(nil), plan.Additions...),
		AffectedController: plan.AffectedController,
		AffectedClients:    append([]string(nil), plan.AffectedClients...),
		Issues:             []domain.ValidationIssue{},
	}
	currentCatalog := m.Presets(ctx, plan.Repository)
	if currentCatalog.HasErrors() || currentCatalog.State != "ready" || currentCatalog.Fingerprint == "" || currentCatalog.Fingerprint != plan.CatalogFingerprint {
		report.State = "conflict"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "catalog", Message: "the deployment software profile catalog changed after review; create a new plan"})
		report.Message = "Software was not changed because the reviewed profile catalog is stale."
		return report
	}
	fresh := m.PlanPreset(ctx, plan.Repository, plan.Request)
	if fresh.HasErrors() {
		report.Issues = append(report.Issues, fresh.Issues...)
		report.Message = fresh.Message
		return report
	}
	if fresh.State == "unchanged" {
		report.State = "unchanged"
		report.Preset = fresh.Preset
		report.Existing = append([]domain.SoftwareDeclaration(nil), fresh.Existing...)
		report.Message = fresh.Message
		return report
	}
	if expectedToken == "" || expectedToken != plan.ReviewToken || expectedToken != fresh.ReviewToken || plan.ManagedFile != fresh.ManagedFile || plan.CatalogFingerprint != fresh.CatalogFingerprint {
		report.State = "conflict"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "reviewToken", Message: "the software profile proposal changed after review; create a new plan"})
		report.Message = "Software was not changed because the reviewed profile proposal is stale."
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
		switch {
		case errors.Is(err, domain.ErrSoftwareConflict):
			report.State = "conflict"
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: "file", Message: "lab-software.json changed while applying; create a new plan"})
			report.Message = "Software was not changed because the managed file changed."
		case errors.Is(err, domain.ErrSoftwareDurability):
			report.State = "partial"
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: "durability", Message: err.Error()})
			report.Message = "lab-software.json was replaced, but durable storage could not be confirmed. Inspect the file and Git state before creating another proposal."
		default:
			report.State = "failed"
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: "write", Message: err.Error()})
			report.Message = "Software profile declarations were not saved."
		}
		return report
	}
	report.State = "applied"
	report.Repository = fresh.Repository
	report.ManagedFile = fresh.ManagedFile
	report.Request = fresh.Request
	report.Preset = fresh.Preset
	report.Existing = append([]domain.SoftwareDeclaration(nil), fresh.Existing...)
	report.Additions = append([]domain.SoftwareDeclaration(nil), fresh.Additions...)
	report.AffectedController = fresh.AffectedController
	report.AffectedClients = append([]string(nil), fresh.AffectedClients...)
	report.Message = "Software profile declarations saved. Review and commit lab-software.json before preparing or distributing systems."
	return report
}

func softwarePresetCatalogIssue(report domain.SoftwarePresetCatalogReport, field, message string) domain.SoftwarePresetCatalogReport {
	report.State = "failed"
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	report.Message = message
	return report
}

func softwarePresetPlanIssue(report domain.SoftwarePresetPlanReport, field, message string) domain.SoftwarePresetPlanReport {
	report.State = "invalid"
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	report.Message = message
	return report
}

func normalizeSoftwarePresetExclusions(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func softwarePresetByID(catalog domain.SoftwarePresetCatalog, id string) (domain.SoftwarePreset, bool) {
	for _, preset := range catalog.Presets {
		if preset.ID == id {
			return preset, true
		}
	}
	return domain.SoftwarePreset{}, false
}
