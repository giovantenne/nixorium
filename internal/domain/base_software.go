package domain

import (
	_ "embed"
	"encoding/json"
	"slices"
)

// Base software is part of every laboratory computer: Nixorium installs it
// whatever lab-software.json declares, because its own features depend on
// it (the browser for the classroom view, the terminal for the Nixorium
// launcher and the screensaver, Git for management, the screensaver's
// text effects). lib/mk-lab.nix reads the same file.
//
//go:embed base-software.json
var baseSoftwareJSON []byte

var baseSoftware = func() []string {
	var packages []string
	if err := json.Unmarshal(baseSoftwareJSON, &packages); err != nil {
		panic("base-software.json: " + err.Error())
	}
	return packages
}()

// BaseSoftware lists the always-installed package attributes.
func BaseSoftware() []string { return slices.Clone(baseSoftware) }

// IsBaseSoftware tells whether a package is always installed.
func IsBaseSoftware(id string) bool { return slices.Contains(baseSoftware, id) }
