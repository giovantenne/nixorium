package presentation

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"
)

type TerminalSecretReader struct {
	Input  *os.File
	Output io.Writer
}

func (r TerminalSecretReader) ReadSecret(prompt string) ([]byte, error) {
	if r.Input == nil || r.Output == nil {
		return nil, errors.New("terminal input and output are required")
	}
	fileDescriptor := r.Input.Fd()
	if !term.IsTerminal(fileDescriptor) {
		return nil, errors.New("refusing to read a password from a non-terminal")
	}
	if _, err := fmt.Fprint(r.Output, prompt); err != nil {
		return nil, err
	}
	password, err := term.ReadPassword(fileDescriptor)
	fmt.Fprintln(r.Output)
	return password, err
}
