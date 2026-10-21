package presentation

import (
	"fmt"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

// hostPicker is the client selection shared by the classroom operations: a
// cursor over the configured clients and the names chosen with Space or a.
type hostPicker struct {
	cursor int
	chosen map[string]bool
}

func newHostPicker() hostPicker { return hostPicker{chosen: map[string]bool{}} }

// clear starts a new selection with nothing chosen, so that "a All" selects
// everyone instead of clearing the previous selection.
func (p *hostPicker) clear() { p.cursor, p.chosen = 0, map[string]bool{} }

// pick handles the selection keys and reports whether key was one of them.
func (p *hostPicker) pick(key string, hosts []domain.HostMeta) bool {
	switch key {
	case "up", "k":
		p.cursor = max(0, p.cursor-1)
	case "down", "j":
		p.cursor = min(max(0, len(hosts)-1), p.cursor+1)
	case "space":
		if len(hosts) > 0 {
			name := hosts[p.cursor].Name
			p.chosen[name] = !p.chosen[name]
		}
	case "a":
		all := true
		for _, host := range hosts {
			all = all && p.chosen[host.Name]
		}
		for _, host := range hosts {
			p.chosen[host.Name] = !all
		}
	default:
		return false
	}
	return true
}

// requested lists the chosen clients in inventory order, comma separated, as
// the plans expect; it is empty when nothing is chosen.
func (p hostPicker) requested(hosts []domain.HostMeta) string {
	names := []string{}
	for _, host := range hosts {
		if p.chosen[host.Name] {
			names = append(names, host.Name)
		}
	}
	return strings.Join(names, ",")
}

// rows renders at most count clients around the cursor; label gives the text
// after the check box.
func (p hostPicker) rows(hosts []domain.HostMeta, count int, dark bool, label func(domain.HostMeta) string) []string {
	if len(hosts) == 0 {
		return []string{"No client computers configured."}
	}
	lines := []string{}
	start, end := listWindow(len(hosts), p.cursor, count)
	for i := start; i < end; i++ {
		mark := "[ ]"
		if p.chosen[hosts[i].Name] {
			mark = "[x]"
		}
		lines = append(lines, tuiSelection(mark+" "+label(hosts[i]), i == p.cursor, dark))
	}
	if start > 0 || end < len(hosts) {
		lines = append(lines, fmt.Sprintf("Showing %d–%d of %d", start+1, end, len(hosts)))
	}
	return lines
}
