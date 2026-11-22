package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// reporter describes planned changes before they are applied. It also works
// with a discarded writer for callers that only need the result.
type reporter struct {
	out     io.Writer
	verbose bool
	style   lipgloss.Style
	changes int
}

func newReporter(out io.Writer, verbose bool) *reporter {
	return &reporter{out: out, verbose: verbose, style: lipgloss.NewRenderer(out).NewStyle().Bold(true).Foreground(lipgloss.Color("6"))}
}

func (r *reporter) progress(format string, args ...any) {
	if r.verbose {
		fmt.Fprintf(r.out, "  %s\n", fmt.Sprintf(format, args...))
	}
}

func (r *reporter) change(action, path, reason string, details []string) {
	r.changes++
	fmt.Fprintf(r.out, "%s %s\n  %s\n", r.style.Render(action), filepath.ToSlash(path), reason)
	for _, detail := range details {
		fmt.Fprintf(r.out, "  %s\n", detail)
	}
}

func (r *reporter) summary(mode string) {
	if r.changes == 0 {
		fmt.Fprintln(r.out, "No changes needed.")
	} else if mode == "check" {
		fmt.Fprintf(r.out, "%d file(s) need changes. Run generate.\n", r.changes)
	} else {
		fmt.Fprintf(r.out, "%d file(s) changed.\n", r.changes)
	}
}

// jsonDifferences compares JSON values so reports name the fields that changed,
// even when formatting also changed. Arrays are displayed as complete values.
func jsonDifferences(before, after []byte) []string {
	var oldValue, newValue any
	if json.Unmarshal(before, &oldValue) != nil || json.Unmarshal(after, &newValue) != nil {
		return []string{"Existing content is missing or invalid JSON; generated content will replace it."}
	}
	var lines []string
	compareJSON("$", oldValue, newValue, &lines)
	if len(lines) == 0 && !bytes.Equal(before, after) {
		return []string{"JSON values are unchanged; generated formatting will be restored."}
	}
	return lines
}

func compareJSON(path string, oldValue, newValue any, lines *[]string) {
	oldMap, oldObject := oldValue.(map[string]any)
	newMap, newObject := newValue.(map[string]any)
	if oldObject && newObject {
		keys := make(map[string]bool)
		for key := range oldMap {
			keys[key] = true
		}
		for key := range newMap {
			keys[key] = true
		}
		names := make([]string, 0, len(keys))
		for key := range keys {
			names = append(names, key)
		}
		sort.Strings(names)
		for _, key := range names {
			field := path + "." + key
			old, hadOld := oldMap[key]
			current, hasNew := newMap[key]
			switch {
			case !hadOld:
				*lines = append(*lines, fmt.Sprintf("+ %s = %s", field, jsonValue(current)))
			case !hasNew:
				*lines = append(*lines, fmt.Sprintf("- %s = %s", field, jsonValue(old)))
			default:
				compareJSON(field, old, current, lines)
			}
		}
		return
	}
	if !reflect.DeepEqual(oldValue, newValue) {
		*lines = append(*lines, fmt.Sprintf("~ %s: %s -> %s", path, jsonValue(oldValue), jsonValue(newValue)))
	}
}

func jsonValue(value any) string {
	data, _ := json.Marshal(value)
	return strings.TrimSpace(string(data))
}
