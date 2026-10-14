// Command nixorium-demo exports deterministic website demo frames from the
// real presentation package. It has no operational adapters and is not part of
// the distributed nixorium command.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/giovantenne/nixorium/internal/presentation"
)

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func main() {
	output := flag.String("output", "", "write the generated JSON to this file (stdout when empty)")
	assistantOutput := flag.String("assistant-output", "", "export localized Assistant pricing ANSI frames to this directory")
	graphOutput := flag.String("graph-output", "", "also write the navigable demo graphs to this file")
	commit := flag.String("source-commit", "", "full Nixorium Git commit used by the demo")
	date := flag.String("source-date", "", "source commit date in YYYY-MM-DD form")
	flag.Parse()
	if !revisionPattern.MatchString(*commit) || !datePattern.MatchString(*date) {
		fmt.Fprintln(os.Stderr, "nixorium-demo: --source-commit and --source-date are required")
		os.Exit(2)
	}
	if *assistantOutput != "" {
		if err := exportAssistant(*assistantOutput, *commit, *date); err != nil {
			fmt.Fprintln(os.Stderr, "nixorium-demo:", err)
			os.Exit(1)
		}
		return
	}
	data, err := json.MarshalIndent(presentation.RenderDemoBundle(*commit, *date), "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "nixorium-demo:", err)
		os.Exit(1)
	}
	if *graphOutput != "" {
		// Compact: the website downloads this file when a visitor opens a demo.
		graphs, err := json.Marshal(presentation.RenderDemoGraphs(*commit, *date))
		if err == nil {
			err = os.WriteFile(*graphOutput, append(graphs, '\n'), 0o644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "nixorium-demo:", err)
			os.Exit(1)
		}
	}
	data = append(data, '\n')
	if *output == "" {
		_, err = os.Stdout.Write(data)
	} else {
		err = os.WriteFile(*output, data, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "nixorium-demo:", err)
		os.Exit(1)
	}
}

func exportAssistant(directory, commit, date string) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	for _, demo := range presentation.RenderAssistantPricingDemos() {
		if err := os.WriteFile(filepath.Join(directory, "assistant."+demo.Locale+".txt"), []byte(demo.Prompt), 0o644); err != nil {
			return err
		}
		for index, frame := range demo.Frames {
			name := fmt.Sprintf("assistant.%s.%03d.ansi", demo.Locale, index)
			if err := os.WriteFile(filepath.Join(directory, name), []byte(frame.ANSI), 0o644); err != nil {
				return err
			}
		}
	}
	manifest, err := json.MarshalIndent(struct {
		SourceCommit string `json:"sourceCommit"`
		SourceDate   string `json:"sourceDate"`
		Terminal     string `json:"terminal"`
		Synthetic    bool   `json:"synthetic"`
	}{commit, date, "84x28", true}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "assistant-manifest.json"), append(manifest, '\n'), 0o644)
}
