// Command nixorium-demo exports deterministic website demo frames from the
// real presentation package. It has no operational adapters and is not part of
// the distributed nixorium command.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"

	"github.com/giovantenne/nixorium/internal/presentation"
)

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func main() {
	output := flag.String("output", "", "write the generated JSON to this file (stdout when empty)")
	commit := flag.String("source-commit", "", "full Nixorium Git commit used by the demo")
	date := flag.String("source-date", "", "source commit date in YYYY-MM-DD form")
	flag.Parse()
	if !revisionPattern.MatchString(*commit) || !datePattern.MatchString(*date) {
		fmt.Fprintln(os.Stderr, "nixorium-demo: --source-commit and --source-date are required")
		os.Exit(2)
	}
	data, err := json.MarshalIndent(presentation.RenderDemoBundle(*commit, *date), "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "nixorium-demo:", err)
		os.Exit(1)
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
