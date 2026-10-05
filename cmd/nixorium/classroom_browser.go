package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// classroomBrowserCommand is the internal command that runs the classroom
// view's browser and serves the user's desktop to the classroom service
// while it is open ("Send files" in the page). It is not for operators.
const classroomBrowserCommand = "__classroom-browser"

// runClassroomBrowser starts the browser and, as the only desktop helper of
// this user, answers the service's desktop jobs until the classroom
// browser has closed.
func runClassroomBrowser(address string) int {
	profile, err := classroomBrowserProfile()
	if err != nil {
		return 1
	}
	arguments := classroomBrowserArguments(address, profile, desktopPrefersDark())
	path, err := exec.LookPath(arguments[0])
	if err != nil {
		return 1
	}
	browser := exec.Command(path, arguments[1:]...)
	if err := browser.Start(); err != nil {
		return 1
	}
	browserDone := make(chan struct{})
	go func() {
		_ = browser.Wait()
		close(browserDone)
	}()
	lock, err := os.OpenFile(filepath.Join(profile, "nixorium-desktop-helper.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		<-browserDone
		return 0
	}
	defer lock.Close()
	// Another helper already serves this user: only open the page.
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		<-browserDone
		return 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-browserDone
		// A new window may have joined a browser that is still running.
		for classroomBrowserRunning(profile) {
			time.Sleep(5 * time.Second)
		}
		cancel()
	}()
	serveDesktopJobs(ctx)
	return 0
}

func serveDesktopJobs(ctx context.Context) {
	for ctx.Err() == nil {
		waitCtx, stop := context.WithTimeout(ctx, time.Minute)
		response, err := classroomRequest(waitCtx, domain.ClassroomDesktopWaitOperation, nil)
		stop()
		if err != nil {
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
			continue
		}
		if response.DesktopJob == "" {
			continue
		}
		transfer, message := "", ""
		item, chosen, chooseErr := chooseShareItem(ctx, response.DesktopKind)
		if chooseErr == nil && chosen {
			transfer, chooseErr = prepareShare(ctx, item)
		}
		if chooseErr != nil {
			message = strings.ToUpper(chooseErr.Error()[:1]) + chooseErr.Error()[1:] + "."
		}
		_, _ = classroomRequest(ctx, domain.ClassroomDesktopReadyOperation, func(request *domain.ClassroomRequest) {
			request.DesktopJob, request.ShareTransfer, request.DesktopError = response.DesktopJob, transfer, message
			request.DesktopCancelled = chooseErr == nil && !chosen
		})
	}
}

// chooseShareItem shows the system file chooser, starting in the home
// folder, for a file or a folder. chosen is false when the user closes it.
func chooseShareItem(ctx context.Context, kind string) (string, bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, err
	}
	arguments := []string{"--file-selection", "--title=Choose a file to send", "--filename=" + home + "/"}
	switch kind {
	case domain.DesktopChooseFile:
	case domain.DesktopChooseFolder:
		arguments = append(arguments, "--directory")
		arguments[1] = "--title=Choose a folder to send"
	default:
		return "", false, errors.New("the page asked for an unknown choice")
	}
	output, err := exec.CommandContext(ctx, "zenity", arguments...).Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.New("the file chooser could not open")
	}
	return strings.TrimSuffix(string(output), "\n"), true, nil
}

// classroomBrowserRunning reads the browser's own singleton lock, a link
// to "host-pid", to tell whether the classroom browser is still open.
func classroomBrowserRunning(profile string) bool {
	target, err := os.Readlink(filepath.Join(profile, "SingletonLock"))
	if err != nil {
		return false
	}
	index := strings.LastIndex(target, "-")
	if index < 0 {
		return false
	}
	pid, err := strconv.Atoi(target[index+1:])
	if err != nil || pid <= 0 {
		return false
	}
	err = syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
