package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// Collect from students' desktops: the files on the desktops of the selected
// students' computers are copied, through the classroom service, into a
// folder of whoever runs Nixorium (teacher or administrator), as that
// user: <folder>/<YYYY-MM-DD HH.MM>/<computer>/…, where the folder is
// ~/Nixorium/Collected unless --into names another existing one. Nothing
// changes on the students' computers. Like the classroom view, it needs
// only the classroom service, not the deployment repository.

// collectApplyTime bounds collecting and saving from the terminal.
const collectApplyTime = 45 * time.Minute

func parseCollectArguments(arguments []string) (options, error) {
	o := options{command: "collect"}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch argument {
		case "plan", "apply":
			if o.subcommand != "" {
				return o, errors.New("only one collect action may be selected")
			}
			o.subcommand = argument
		case "--on", "--into", "--expect":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return o, fmt.Errorf("%s requires a value", argument)
			}
			switch argument {
			case "--on":
				o.on = arguments[index]
			case "--into":
				o.collectInto = arguments[index]
			default:
				o.expect = arguments[index]
			}
		case "--json":
			o.json = true
		case "--yes":
			o.yes = true
		case "-h", "--help", "help":
			o.help = true
		default:
			return o, fmt.Errorf("unknown collect argument %q", argument)
		}
	}
	if o.help {
		return o, nil
	}
	if (o.subcommand != "plan" && o.subcommand != "apply") || o.on == "" {
		return o, errors.New("collect requires plan/apply and --on")
	}
	if o.subcommand == "apply" && o.expect == "" {
		return o, errors.New("collect apply requires --expect from collect plan")
	}
	if o.subcommand == "plan" && (o.expect != "" || o.yes) {
		return o, errors.New("--expect and --yes are only valid with collect apply")
	}
	return o, nil
}

// runCollectCommand reviews or applies a collection. Apply reviews again
// and needs the same review token, which binds the computers and the
// folder.
func runCollectCommand(ctx context.Context, options options, stdout, stderr io.Writer) int {
	plan := planCollect(ctx, options.on, options.collectInto)
	if options.subcommand == "plan" || plan.HasErrors() {
		if options.json {
			_ = presentation.JSON(stdout, plan)
		} else {
			presentation.CollectPlanText(stdout, plan)
		}
		if plan.HasErrors() {
			return 1
		}
		return 0
	}
	if options.expect != plan.ReviewToken {
		fmt.Fprintln(stderr, "The computers or the folder changed since the review; create a fresh plan.")
		return 1
	}
	if !options.yes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Collect apply requires an interactive terminal or --yes.")
			return 2
		}
		approved, err := presentation.ConfirmCollect(os.Stdin, stderr, plan)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if !approved {
			return 0
		}
	}
	report := applyCollect(ctx, plan)
	if options.json {
		_ = presentation.JSON(stdout, report)
	} else {
		presentation.CollectReportText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func collectFailure(message string) domain.CollectPlan {
	return domain.CollectPlan{SchemaVersion: domain.SchemaVersion, Operation: "collect-plan", State: "blocked", Message: message, Targets: []domain.CollectTarget{}, Issues: []domain.ValidationIssue{{Field: "collect", Message: message}}}
}

// collectDestination is the absolute path of an existing folder, or of
// ~/Nixorium/Collected (created when the files are saved) without one.
func collectDestination(folder string) (string, error) {
	if folder == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, filepath.FromSlash(saveFolders[domain.DesktopSaveCollected])), nil
	}
	absolute, err := filepath.Abs(folder)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not an existing folder", absolute)
	}
	return absolute, nil
}

func planCollect(ctx context.Context, requested, folder string) domain.CollectPlan {
	destination, err := collectDestination(folder)
	if err != nil {
		return collectFailure(err.Error() + ".")
	}
	response, err := classroomCall(ctx, domain.ClassroomCollectPlanOperation, func(request *domain.ClassroomRequest) {
		request.Requested, request.CollectFolder = requested, destination
	})
	if err != nil || response.CollectPlan == nil {
		return collectFailure(classroomMessage(response, err, "Collecting files is unavailable."))
	}
	return *response.CollectPlan
}

// applyCollect collects the reviewed computers and saves the files into
// the reviewed folder.
func applyCollect(ctx context.Context, plan domain.CollectPlan) domain.CollectReport {
	ctx, cancel := context.WithTimeout(ctx, collectApplyTime)
	defer cancel()
	response, err := classroomCall(ctx, domain.ClassroomCollectApplyOperation, func(request *domain.ClassroomRequest) {
		request.CollectPlan = &plan
	})
	if err != nil || response.CollectReport == nil {
		return domain.CollectReport{SchemaVersion: domain.SchemaVersion, Operation: "collect-apply", State: "blocked", Targets: []domain.CollectOutcome{}, Message: classroomMessage(response, err, "The reviewed collection could not be applied.")}
	}
	report := *response.CollectReport
	if report.Save == "" {
		return report
	}
	save := report.Save
	report.Save = ""
	err = classroomview.EnsureFolder(plan.Destination)
	var placed []string
	if err == nil {
		_, placed, err = receiveSave(ctx, save, plan.Destination)
	}
	names, message := []string{}, ""
	if err == nil && len(placed) != 1 {
		err = errors.New("the collected files arrived incomplete")
	}
	if err != nil {
		message = err.Error()
		report.State, report.Folder = "partial", ""
		report.Message += " The files could not be saved in " + plan.Destination + ": " + message + "."
		for index := range report.Targets {
			if report.Targets[index].State == domain.CollectCollected {
				report.Targets[index].State, report.Targets[index].Detail = domain.CollectFailed, "Collected, but not saved."
			}
		}
	} else {
		names = placed
		report.Folder = filepath.Join(plan.Destination, placed[0])
		report.Message += " Saved in " + report.Folder + "."
	}
	_, _ = classroomCall(ctx, domain.ClassroomSaveDoneOperation, func(request *domain.ClassroomRequest) {
		request.DesktopJob, request.SaveNames, request.DesktopError = save, names, message
	})
	return report
}
