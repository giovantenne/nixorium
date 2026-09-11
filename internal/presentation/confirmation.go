package presentation

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
)

func IsInteractive(input *os.File) bool {
	return input != nil && term.IsTerminal(input.Fd())
}

func ConfirmControllerApply(input io.Reader, output io.Writer, controller string) (bool, error) {
	fmt.Fprintf(output, "Controller apply review\n")
	fmt.Fprintf(output, "Machine: %s (this controller only)\n", controller)
	fmt.Fprintln(output, "Action: validate, build, and activate the reviewed Git configuration")
	fmt.Fprintln(output, "Impact: services and networking may restart; this terminal connection may be interrupted")
	fmt.Fprintln(output, "Reboot: not normally required")
	fmt.Fprint(output, "Type APPLY to continue: ")
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(value) == 0 {
		return false, err
	}
	return strings.TrimSpace(value) == "APPLY", nil
}
