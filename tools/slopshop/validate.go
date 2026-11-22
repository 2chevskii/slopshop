package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hashicorp/hcl/v2"
)

var errInvalidManifest = errors.New("invalid slopshop.hcl")

type validationResult struct {
	Valid       bool                   `json:"valid"`
	Diagnostics []validationDiagnostic `json:"diagnostics"`
}

type validationDiagnostic struct {
	Severity string           `json:"severity"`
	Summary  string           `json:"summary"`
	Detail   string           `json:"detail"`
	Range    *diagnosticRange `json:"range,omitempty"`
}

type diagnosticRange struct {
	File  string             `json:"file"`
	Start diagnosticPosition `json:"start"`
	End   diagnosticPosition `json:"end"`
}

type diagnosticPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
	Byte   int `json:"byte"`
}

func validateCommand(root, output string, verbose bool, stdout, stderr io.Writer) error {
	switch output {
	case "rich", "json", "plain":
	default:
		return fmt.Errorf("invalid output format %q (use rich, json, or plain)", output)
	}

	progress := stdout
	if output == "json" {
		progress = stderr
	}
	newReporter(progress, verbose).progress("Reading slopshop.hcl")
	_, file, diagnostics := readManifest(root)
	valid := !diagnostics.HasErrors()
	if output == "json" {
		if err := writeValidationJSON(stdout, valid, diagnostics); err != nil {
			return err
		}
	} else {
		if err := writeValidationText(stderr, output == "rich", file, diagnostics); err != nil {
			return err
		}
		if valid {
			_, err := fmt.Fprintln(stdout, "slopshop.hcl is valid.")
			return err
		}
	}
	if !valid {
		return errInvalidManifest
	}
	return nil
}

func writeValidationJSON(out io.Writer, valid bool, diagnostics hcl.Diagnostics) error {
	result := validationResult{
		Valid:       valid,
		Diagnostics: make([]validationDiagnostic, 0, len(diagnostics)),
	}
	for _, diagnostic := range diagnostics {
		item := validationDiagnostic{
			Severity: severityName(diagnostic),
			Summary:  diagnostic.Summary,
			Detail:   diagnostic.Detail,
		}
		if subject := diagnostic.Subject; subject != nil {
			item.Range = &diagnosticRange{
				File:  subject.Filename,
				Start: diagnosticPosition(subject.Start),
				End:   diagnosticPosition(subject.End),
			}
		}
		result.Diagnostics = append(result.Diagnostics, item)
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func writeValidationText(out io.Writer, rich bool, file *hcl.File, diagnostics hcl.Diagnostics) error {
	renderer := lipgloss.NewRenderer(out)
	for i, diagnostic := range diagnostics {
		var message strings.Builder
		if rich && i > 0 {
			message.WriteByte('\n')
		}
		subject := diagnostic.Subject
		if subject != nil {
			fmt.Fprintf(&message, "%s:%d:%d: ", subject.Filename, subject.Start.Line, subject.Start.Column)
		}
		severity := severityName(diagnostic)
		if rich {
			color := "1"
			if diagnostic.Severity == hcl.DiagWarning {
				color = "3"
			}
			severity = renderer.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(severity)
		}
		fmt.Fprintf(&message, "%s: %s", severity, diagnostic.Summary)
		if rich {
			message.WriteByte('\n')
			if subject != nil && file != nil {
				writeSourceExcerpt(&message, file.Bytes, *subject)
			}
			if diagnostic.Detail != "" {
				fmt.Fprintf(&message, "  %s\n", diagnostic.Detail)
			}
		} else {
			if diagnostic.Detail != "" {
				detail := strings.Join(strings.Fields(diagnostic.Detail), " ")
				fmt.Fprintf(&message, ": %s", detail)
			}
			message.WriteByte('\n')
		}
		if _, err := io.WriteString(out, message.String()); err != nil {
			return err
		}
	}
	return nil
}

func writeSourceExcerpt(out *strings.Builder, source []byte, subject hcl.Range) {
	start := subject.Start.Byte
	if start < 0 || start > len(source) {
		return
	}
	lineStart := bytes.LastIndexByte(source[:start], '\n') + 1
	lineEnd := len(source)
	if offset := bytes.IndexByte(source[start:], '\n'); offset >= 0 {
		lineEnd = start + offset
	}
	line := strings.TrimSuffix(string(source[lineStart:lineEnd]), "\r")
	line = strings.ReplaceAll(line, "\t", "    ")
	prefix := strings.ReplaceAll(string(source[lineStart:start]), "\t", "    ")
	end := min(max(start, subject.End.Byte), lineEnd)
	highlight := strings.ReplaceAll(string(source[start:end]), "\t", "    ")
	caretOffset := lipgloss.Width(prefix)
	caretWidth := max(1, lipgloss.Width(highlight))

	lineNumber := fmt.Sprint(subject.Start.Line)
	fmt.Fprintf(out, "  %s | %s\n", lineNumber, line)
	fmt.Fprintf(out, "  %s | %s%s\n", strings.Repeat(" ", len(lineNumber)), strings.Repeat(" ", caretOffset), strings.Repeat("^", caretWidth))
}

func severityName(diagnostic *hcl.Diagnostic) string {
	if diagnostic.Severity == hcl.DiagWarning {
		return "warning"
	}
	return "error"
}
