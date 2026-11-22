package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/pmezard/go-difflib/difflib"
)

// reporter describes planned changes before they are applied. It also works
// with a discarded writer for callers that only need the result.
type reporter struct {
	out      io.Writer
	verbose  bool
	renderer *lipgloss.Renderer
	changes  int
}

func newReporter(out io.Writer, verbose bool) *reporter {
	return &reporter{out: out, verbose: verbose, renderer: lipgloss.NewRenderer(out)}
}

func (r *reporter) progress(format string, args ...any) {
	if r.verbose {
		fmt.Fprintf(r.out, "  %s\n", fmt.Sprintf(format, args...))
	}
}

func (r *reporter) change(action, path, reason string, details []string) {
	r.changes++
	fmt.Fprintf(r.out, "%s %s\n  %s\n", r.changeMarker(action), filepath.ToSlash(path), reason)
	for _, detail := range details {
		fmt.Fprintf(r.out, "  %s\n", detail)
	}
}

func (r *reporter) changeMarker(marker string) string {
	var color lipgloss.Color
	switch marker {
	case "CREATE", "+":
		color = "2"
	case "REMOVE", "-":
		color = "1"
	case "UPDATE", "~":
		color = "3"
	default:
		return marker
	}
	return r.renderer.NewStyle().Bold(true).Foreground(color).Render(marker)
}

func (r *reporter) textDifferences(before, after []byte) []string {
	if !utf8.Valid(before) || !utf8.Valid(after) || bytes.IndexByte(before, 0) >= 0 || bytes.IndexByte(after, 0) >= 0 {
		return []string{"Binary content changed."}
	}
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A: difflib.SplitLines(string(before)), B: difflib.SplitLines(string(after)),
		FromFile: "previous", ToFile: "updated", Context: 3,
	})
	if err != nil || diff == "" {
		return []string{"File content changed."}
	}
	lines := strings.Split(strings.TrimRight(diff, "\n"), "\n")
	for i, line := range lines {
		switch {
		case i < 2, strings.HasPrefix(line, "@@"):
			lines[i] = r.renderer.NewStyle().Foreground(lipgloss.Color("6")).Render(line)
		case strings.HasPrefix(line, "+"), strings.HasPrefix(line, "-"):
			lines[i] = r.changeMarker(line[:1]) + line[1:]
		}
	}
	return lines
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
func (r *reporter) jsonDifferences(before, after []byte) []string {
	var oldValue, newValue any
	if json.Unmarshal(before, &oldValue) != nil || json.Unmarshal(after, &newValue) != nil {
		return []string{"Existing content is missing or invalid JSON; generated content will replace it."}
	}
	var lines []string
	r.compareJSON("$", oldValue, newValue, &lines)
	if len(lines) == 0 && !bytes.Equal(before, after) {
		return []string{"JSON values are unchanged; generated formatting will be restored."}
	}
	return lines
}

func (r *reporter) compareJSON(path string, oldValue, newValue any, lines *[]string) {
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
				*lines = append(*lines, fmt.Sprintf("%s %s = %s", r.changeMarker("+"), r.jsonPath(field), r.jsonValue(current)))
			case !hasNew:
				*lines = append(*lines, fmt.Sprintf("%s %s = %s", r.changeMarker("-"), r.jsonPath(field), r.jsonValue(old)))
			default:
				r.compareJSON(field, old, current, lines)
			}
		}
		return
	}
	if !reflect.DeepEqual(oldValue, newValue) {
		*lines = append(*lines, fmt.Sprintf("%s %s: %s -> %s", r.changeMarker("~"), r.jsonPath(path), r.jsonValue(oldValue), r.jsonValue(newValue)))
	}
}

func (r *reporter) jsonPath(path string) string {
	return r.renderer.NewStyle().Foreground(lipgloss.Color("6")).Render(path)
}

// Values are already valid, compact JSON, so a string followed by a colon is an object key.
var jsonToken = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|-?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?|true|false|null`)

func (r *reporter) jsonValue(value any) string {
	data, _ := json.Marshal(value)
	text := string(data)
	var output strings.Builder
	offset := 0
	for _, match := range jsonToken.FindAllStringIndex(text, -1) {
		start, end := match[0], match[1]
		token := text[start:end]
		var color lipgloss.Color
		switch {
		case token[0] == '"' && end < len(text) && text[end] == ':':
			color = "4"
		case token[0] == '"':
			color = "2"
		case token == "true" || token == "false" || token == "null":
			color = "5"
		default:
			color = "3"
		}
		output.WriteString(text[offset:start])
		output.WriteString(r.renderer.NewStyle().Foreground(color).Render(token))
		offset = end
	}
	output.WriteString(text[offset:])
	return output.String()
}
