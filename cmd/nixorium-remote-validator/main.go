package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/giovantenne/nixorium/internal/domain"
)

var nixoriumVersion = "development"

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: nixorium-remote-validator < plan.json")
		os.Exit(2)
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, domain.RemoteInstallPlanMaxBytes+1))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read remote installation plan:", err)
		os.Exit(2)
	}
	plan, err := domain.DecodeRemoteInstallPlan(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(plan); err != nil {
		fmt.Fprintln(os.Stderr, "encode validated remote installation plan:", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(output.Bytes()); err != nil {
		fmt.Fprintln(os.Stderr, "write validated remote installation plan:", err)
		os.Exit(1)
	}
	_ = nixoriumVersion
}
