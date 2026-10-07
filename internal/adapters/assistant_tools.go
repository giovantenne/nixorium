package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/giovantenne/nixorium/internal/domain"
)

type AssistantFileTools struct{}

func assistantToolResult(value any) string {
	data, _ := json.Marshal(value)
	return domain.RedactForModel(string(data))
}
func assistantToolFailure(err error) string {
	if errors.Is(err, errAssistantPathCollision) {
		return assistantToolResult(map[string]any{"ok": false, "error": "invalid_arguments", "message": err.Error()})
	}
	code := "invalid_arguments"
	switch {
	case errors.Is(err, fs.ErrPermission):
		code = "protected_path"
	case errors.Is(err, fs.ErrNotExist):
		code = "not_found"
	case err != nil && (err.Error() == "not_text" || err.Error() == "limit_exceeded"):
		code = err.Error()
	}
	return assistantToolResult(map[string]any{"ok": false, "error": code, "message": "The requested tool operation could not be completed."})
}
func (AssistantFileTools) Execute(ctx context.Context, workspace *AssistantWorkspace, call domain.AssistantToolCall) string {
	if ctx.Err() != nil {
		return assistantToolResult(map[string]any{"ok": false, "error": "timeout", "message": "The request was stopped."})
	}
	if !domain.ValidateAssistantArguments(call.Name, call.Arguments) {
		return assistantToolFailure(nil)
	}
	var args struct {
		Path       string
		Pattern    string
		Content    string
		Offset     int
		Limit      int
		Executable bool
	}
	if json.Unmarshal(call.Arguments, &args) != nil {
		return assistantToolFailure(nil)
	}
	if args.Path != "" && (!domain.ValidAssistantPath(args.Path) || domain.AssistantHiddenPath(args.Path)) {
		return assistantToolFailure(fs.ErrPermission)
	}
	switch call.Name {
	case "list_files":
		entries, err := workspace.Entries()
		if err != nil {
			return assistantToolFailure(err)
		}
		visible := map[string]AssistantFileEntry{}
		for _, entry := range entries {
			if args.Path != "" && entry.Path != args.Path && !strings.HasPrefix(entry.Path, args.Path+"/") {
				continue
			}
			visible[entry.Path] = entry
			for parent := filepath.ToSlash(filepath.Dir(entry.Path)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
				if args.Path != "" && parent != args.Path && !strings.HasPrefix(parent, args.Path+"/") {
					continue
				}
				visible[parent] = AssistantFileEntry{parent, "dir", 0}
			}
		}
		if len(visible) > 2000 {
			return assistantToolFailure(errors.New("limit_exceeded"))
		}
		result := make([]AssistantFileEntry, 0, len(visible))
		for _, entry := range visible {
			result = append(result, entry)
		}
		sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
		return assistantToolResult(map[string]any{"ok": true, "entries": result})
	case "read_file":
		content, err := workspace.Read(args.Path)
		if err != nil {
			return assistantToolFailure(err)
		}
		limit := args.Limit
		if limit == 0 {
			limit = 65536
		}
		if args.Offset > len(content) {
			return assistantToolFailure(nil)
		}
		end := min(len(content), args.Offset+limit)
		// Windows are byte-based and never emit broken UTF-8 at either edge.
		start := args.Offset
		for start < end && !utf8.RuneStart(content[start]) {
			start++
		}
		for end < len(content) && end > start && !utf8.RuneStart(content[end]) {
			end--
		}
		return assistantToolResult(map[string]any{"ok": true, "content": string(content[start:end]), "totalBytes": len(content), "truncated": start > 0 || end < len(content)})
	case "search":
		pattern, err := regexp.Compile(args.Pattern)
		if err != nil {
			return assistantToolFailure(nil)
		}
		entries, err := workspace.Entries()
		if err != nil {
			return assistantToolFailure(err)
		}
		matches := []map[string]any{}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return assistantToolResult(map[string]any{"ok": false, "error": "timeout"})
			}
			if args.Path != "" && entry.Path != args.Path && !strings.HasPrefix(entry.Path, args.Path+"/") {
				continue
			}
			content, err := workspace.Read(entry.Path)
			if err != nil {
				if err.Error() == "not_text" {
					continue
				}
				return assistantToolFailure(err)
			}
			for line, text := range strings.Split(string(content), "\n") {
				if pattern.MatchString(text) {
					runes := []rune(text)
					if len(runes) > 300 {
						runes = runes[:300]
					}
					matches = append(matches, map[string]any{"path": entry.Path, "line": line + 1, "text": string(runes)})
					if len(matches) >= 200 {
						return assistantToolResult(map[string]any{"ok": true, "matches": matches})
					}
				}
			}
		}
		return assistantToolResult(map[string]any{"ok": true, "matches": matches})
	case "write_file":
		if err := workspace.Write(args.Path, args.Content, args.Executable); err != nil {
			return assistantToolFailure(err)
		}
		return assistantToolResult(map[string]any{"ok": true})
	case "delete_file":
		if err := workspace.Delete(args.Path); err != nil {
			return assistantToolFailure(err)
		}
		return assistantToolResult(map[string]any{"ok": true})
	}
	return assistantToolFailure(nil)
}
