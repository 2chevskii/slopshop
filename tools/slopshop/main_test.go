package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func executeCommand(root string, args ...string) (string, error) {
	var output bytes.Buffer
	command := newCommand(root, &output, &output)
	command.SetArgs(args)
	err := command.Execute()
	return output.String(), err
}

func TestCommandReportsChangesAndVerboseProgress(t *testing.T) {
	root := fixture(t)
	output, err := executeCommand(root, "generate", "--verbose")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Reading slopshop.hcl", "CREATE", "plugins/firefox/mcp.json", "slopshop.hcl", "$.mcpServers"} {
		if !strings.Contains(output, text) {
			t.Errorf("missing %q in output:\n%s", text, output)
		}
	}
	output, err = executeCommand(root, "--verbose", "check")
	if err != nil || !strings.Contains(output, "Unchanged:") || !strings.Contains(output, "No changes needed.") {
		t.Fatalf("check: %v\n%s", err, output)
	}
	put(t, root, "plugins/firefox/mcp.json", "{}\n")
	put(t, root, "plugins/firefox/plugin.json", "{}\n")
	output, err = executeCommand(root, "check")
	if err == nil {
		t.Fatal("check must fail for stale files")
	}
	for _, text := range []string{"plugins/firefox/mcp.json", "plugins/firefox/plugin.json", "$.mcpServers", "$.version"} {
		if !strings.Contains(output, text) {
			t.Errorf("missing %q in output:\n%s", text, output)
		}
	}
	if strings.Contains(output, "Reading slopshop.hcl") {
		t.Error("progress appeared without --verbose")
	}
	data, err := os.ReadFile(filepath.Join(root, "plugins", "firefox", "mcp.json"))
	if err != nil || string(data) != "{}\n" {
		t.Fatalf("check modified a file: %q %v", data, err)
	}
}

func TestCommandHelpAndArguments(t *testing.T) {
	root := fixture(t)
	output, err := executeCommand(root, "--help")
	if err != nil || !strings.Contains(output, "--verbose") || !strings.Contains(output, "generate") {
		t.Fatalf("help: %v\n%s", err, output)
	}
	if _, err := executeCommand(root, "generate", "unexpected"); err == nil {
		t.Fatal("unexpected positional argument accepted")
	}
}

func TestObsoleteMCPReport(t *testing.T) {
	root := fixture(t)
	mustRun(t, root, "generate")
	withoutServer := strings.Replace(testManifest, ` mcp_server "foxmcp" {
  type = "stdio"
  command = "npx"
  args = ["--yes", "example"]
  env = { EXAMPLE = "value" }
 }
`, "", 1)
	put(t, root, "slopshop.hcl", withoutServer)
	output, err := executeCommand(root, "check")
	if err == nil || !strings.Contains(output, "REMOVE plugins/firefox/mcp.json") || !strings.Contains(output, "no mcp_server blocks") {
		t.Fatalf("check: %v\n%s", err, output)
	}
	path := filepath.Join(root, "plugins", "firefox", "mcp.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("check removed file: %v", err)
	}
	if _, err := executeCommand(root, "generate"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}

func TestRemovedPluginOutputs(t *testing.T) {
	root := fixture(t)
	mustRun(t, root, "generate")
	put(t, root, "plugins/firefox/notes.txt", "keep")
	put(t, root, "plugins/unlisted/plugin.json", "{}")
	manifest, _, _ := strings.Cut(testManifest, `plugin "firefox"`)
	put(t, root, "slopshop.hcl", manifest)
	output, err := executeCommand(root, "check")
	if err == nil || !strings.Contains(output, "REMOVE plugins/firefox/plugin.json") ||
		!strings.Contains(output, "REMOVE plugins/firefox/mcp.json") {
		t.Fatalf("check: %v\n%s", err, output)
	}
	for _, name := range []string{"plugin.json", "mcp.json"} {
		if _, err := os.Stat(filepath.Join(root, "plugins", "firefox", name)); err != nil {
			t.Fatalf("check modified %s: %v", name, err)
		}
	}
	mustRun(t, root, "generate")
	mustRun(t, root, "check")
	for _, name := range []string{"plugin.json", "mcp.json"} {
		if _, err := os.Stat(filepath.Join(root, "plugins", "firefox", name)); !os.IsNotExist(err) {
			t.Fatalf("expected removal of %s: %v", name, err)
		}
	}
	for _, name := range []string{"plugins/firefox/notes.txt", "plugins/unlisted/plugin.json"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("generate modified %s: %v", name, err)
		}
	}
}

func TestJSONDifferences(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	report := newReporter(&bytes.Buffer{}, false)
	diff := strings.Join(report.jsonDifferences([]byte(`{"version":"1","removed":true}`), []byte(`{"version":"2","added":1}`)), "\n")
	for _, part := range []string{`+ $.added = 1`, `- $.removed = true`, `~ $.version: "1" -> "2"`} {
		if !strings.Contains(diff, part) {
			t.Errorf("missing %q in %s", part, diff)
		}
	}
	if diff := report.jsonDifferences([]byte("{\"a\":1}"), []byte("{ \"a\": 1 }")); len(diff) != 1 || !strings.Contains(diff[0], "formatting") {
		t.Fatalf("formatting: %v", diff)
	}
}

func TestCheckHighlightsChanges(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("NO_COLOR", "")
	root := fixture(t)
	mustRun(t, root, "generate")
	put(t, root, "plugins/firefox/plugin.json", `{"version":"0.0.1","obsolete":true}`)
	output, err := executeCommand(root, "check")
	if err == nil {
		t.Fatal("check accepted stale output")
	}
	for _, styled := range []string{
		"\x1b[1;32m+", "\x1b[1;31m-", "\x1b[1;33m~",
		"\x1b[1;33mUPDATE", "\x1b[36m$.version", "\x1b[32m\"0.1.0\"", "\x1b[35mtrue",
	} {
		if !strings.Contains(output, styled) {
			t.Errorf("missing %q in %q", styled, output)
		}
	}

	report := newReporter(&bytes.Buffer{}, false)
	nested := report.jsonValue([]any{map[string]any{"key": "a\"b"}, 12.5, nil})
	for _, styled := range []string{"\x1b[34m\"key\"", "\x1b[32m\"a\\\"b\"", "\x1b[33m12.5", "\x1b[35mnull"} {
		if !strings.Contains(nested, styled) {
			t.Errorf("missing %q in %q", styled, nested)
		}
	}
}

const testManifest = `
marketplace {
 name = "test"
 display_name = "Test"
}
plugin "firefox" {
 version = "0.1.0"
 description = "A plugin"
 mcp_server "foxmcp" {
  type = "stdio"
  command = "npx"
  args = ["--yes", "example"]
  env = { EXAMPLE = "value" }
 }
 skill "local-skill" { path = "local-skill" }
}
`

const remoteSkillBlock = `skill "remote-skill" {
  source = "https://github.com/example/repo"
  path = "skills/remote-skill"
  ref = "v1"
}`

func put(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	put(t, root, "slopshop.hcl", testManifest)
	put(t, root, "skills-source/local-skill/_SKILL.md", "---\nname: local-skill\ndescription: Local skill\n---\n\nContent\n")
	return root
}

func mustRun(t *testing.T, root, mode string) {
	t.Helper()
	if err := run(root, mode); err != nil {
		t.Fatalf("%s: %v", mode, err)
	}
}

func expectError(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Fatalf("expected error containing %q, got %v", contains, err)
	}
}

func TestGenerateAndCheck(t *testing.T) {
	root := fixture(t)
	mustRun(t, root, "generate")
	mustRun(t, root, "check")
	data, err := os.ReadFile(filepath.Join(root, "plugins/firefox/mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err = json.Unmarshal(data, &actual); err != nil {
		t.Fatal(err)
	}
	server := actual.Servers["foxmcp"]
	if server["command"] != "npx" || server["env"].(map[string]any)["EXAMPLE"] != "value" {
		t.Fatalf("unexpected server: %#v", server)
	}
	put(t, root, "plugins/firefox/mcp.json", "{}\n")
	expectError(t, run(root, "check"), "out of date")
	mustRun(t, root, "generate")
	mustRun(t, root, "check")
	put(t, root, "plugins/firefox/skills/unlisted/SKILL.md", "---\nname: unlisted\ndescription: Bad\n---\n")
	expectError(t, run(root, "check"), "undeclared")
}

func TestLocalSkillLifecycle(t *testing.T) {
	root := fixture(t)
	put(t, root, "skills-source/local-skill/reference.txt", "original")
	output, err := executeCommand(root, "check")
	if err == nil || !strings.Contains(output, "CREATE plugins/firefox/skills/local-skill/SKILL.md") {
		t.Fatalf("check: %v\n%s", err, output)
	}
	target := filepath.Join(root, "plugins/firefox/skills/local-skill/reference.txt")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("check created a copy: %v", err)
	}
	mustRun(t, root, "generate")
	mustRun(t, root, "check")
	sourceSkill := filepath.Join(root, "skills-source/local-skill/_SKILL.md")
	generatedSkill := filepath.Join(root, "plugins/firefox/skills/local-skill/SKILL.md")
	sourceData, err := os.ReadFile(sourceSkill)
	if err != nil {
		t.Fatal(err)
	}
	generatedData, err := os.ReadFile(generatedSkill)
	if err != nil || !bytes.Equal(sourceData, generatedData) {
		t.Fatalf("generated SKILL.md differs from source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "plugins/firefox/skills/local-skill/_SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("source filename leaked into generated skill: %v", err)
	}
	put(t, root, "skills-source/local-skill/_SKILL.md", "---\nname: local-skill\ndescription: Updated skill\n---\n")
	output, err = executeCommand(root, "check")
	if err == nil || !strings.Contains(output, "Updated skill") || !strings.Contains(output, "plugins/firefox/skills/local-skill/SKILL.md") {
		t.Fatalf("changed source not reported: %v\n%s", err, output)
	}
	mustRun(t, root, "generate")
	put(t, root, "skills-source/local-skill/reference.txt", "updated")
	output, err = executeCommand(root, "check")
	if err == nil || !strings.Contains(output, "-original") || !strings.Contains(output, "+updated") {
		t.Fatalf("check: %v\n%s", err, output)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "original" {
		t.Fatalf("check modified copy: %q %v", data, err)
	}
	mustRun(t, root, "generate")
	data, err = os.ReadFile(target)
	if err != nil || string(data) != "updated" {
		t.Fatalf("copy not updated: %q %v", data, err)
	}
	put(t, root, "slopshop.hcl", strings.Replace(testManifest, `skill "local-skill" { path = "local-skill" }`, "", 1))
	output, err = executeCommand(root, "check")
	if err == nil || !strings.Contains(output, "REMOVE plugins/firefox/skills/local-skill/SKILL.md") {
		t.Fatalf("check: %v\n%s", err, output)
	}
	mustRun(t, root, "generate")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("generated skill still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "skills-source/local-skill/_SKILL.md")); err != nil {
		t.Fatalf("source removed: %v", err)
	}
	mustRun(t, root, "check")
}

func TestRejectDiscoverableSkillSources(t *testing.T) {
	for _, name := range []string{"skills-source/SKILL.md", "skills-source/unlisted/nested/skill.md"} {
		t.Run(name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, name, "unintended skill")
			for _, mode := range []string{"generate", "check"} {
				expectError(t, run(root, mode), "rename it to _SKILL.md")
			}
		})
	}
}

func TestLocalSourceRequiresUnderscoreSkillFile(t *testing.T) {
	root := fixture(t)
	if err := os.Remove(filepath.Join(root, "skills-source/local-skill/_SKILL.md")); err != nil {
		t.Fatal(err)
	}
	expectError(t, run(root, "generate"), "expected _SKILL.md")
}

func TestLocalSourcePathAndProtection(t *testing.T) {
	root := fixture(t)
	put(t, root, "slopshop.hcl", strings.Replace(testManifest, `path = "local-skill"`, `path = "group/another-name"`, 1))
	put(t, root, "skills-source/group/another-name/_SKILL.md", "---\nname: local-skill\ndescription: Test\n---\n")
	mustRun(t, root, "generate")
	mustRun(t, root, "check")
	put(t, root, "plugins/firefox/skills/local-skill/SKILL.md", "edited generated copy")
	put(t, root, "slopshop.hcl", strings.Replace(testManifest, `skill "local-skill" { path = "local-skill" }`, "", 1))
	expectError(t, run(root, "generate"), "local changes")
}

func TestInvalidManifest(t *testing.T) {
	cases := []struct{ name, manifest, want string }{
		{"duplicate skill", strings.Replace(testManifest, `skill "local-skill" { path = "local-skill" }`, "skill \"local-skill\" { path = \"local-skill\" }\n skill \"local-skill\" { path = \"local-skill\" }", 1), "Duplicate skill"},
		{"dynamic expression", strings.Replace(testManifest, `command = "npx"`, `command = var.command`, 1), "static values only"},
		{"legacy SSE transport", strings.Replace(testManifest, `type = "stdio"`, `type = "sse"`, 1), "Invalid MCP server type"},
		{"portable HTTP spelling in HCL", strings.Replace(testManifest, `type = "stdio"`, `type = "streamable-http"`, 1), "Invalid MCP server type"},
		{"path traversal", strings.Replace(testManifest, `plugin "firefox"`, `plugin "../firefox"`, 1), "Invalid plugin name"},
		{"missing skill path", strings.Replace(testManifest, `{ path = "local-skill" }`, `{}`, 1), "path"},
		{"skill path traversal", strings.Replace(testManifest, `path = "local-skill"`, `path = "../outside"`, 1), "invalid skill path"},
		{"local ref", strings.Replace(testManifest, `{ path = "local-skill" }`, "{\n path = \"local-skill\"\n ref = \"main\"\n}", 1), "ref without source"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, "slopshop.hcl", tc.manifest)
			expectError(t, run(root, "generate"), tc.want)
		})
	}
}

func TestValidateAggregatesManifestErrors(t *testing.T) {
	root := t.TempDir()
	put(t, root, "slopshop.hcl", `
marketplace {
  name = "Bad Name"
  display_name = []
  extra = true
}
marketplace {
  name = "second"
}
plugin "bad-plugin" {
  description = " "
  mcp_server "test" {
    type = false
    command = var.command
    args = make_args()
    first {}
    second {}
  }
  mcp_server "test" {}
  skill "shared" {
    path = "../outside"
    source = "http://example.com/example"
    ref = "-invalid"
  }
}
plugin "bad-plugin" {
  version = ""
  description = "Second plugin"
  skill "shared" {
    ref = "main"
  }
}
plugin {
  version = "1.0.0"
  description = ""
}
`)
	output, err := executeCommand(root, "validate", "--output", "plain")
	if err == nil {
		t.Fatal("validate accepted an invalid manifest")
	}
	for _, want := range []string{
		"slopshop.hcl:3:10: error: Invalid marketplace name", "Unsuitable value type", "Unsupported argument",
		"Duplicate marketplace block", `"display_name" is required`, `"version" is required`,
		"Empty description", "Invalid MCP server type", "Variables not allowed", "Function calls not allowed",
		`configure "first"`, `configure "second"`, "Duplicate mcp_server", "Missing MCP server type",
		"Invalid skill path", "Invalid skill source",
		"cannot start with a hyphen", "Duplicate plugin", "Invalid plugin version", "Duplicate skill",
		`"path" is required`, "ref without source", "Missing name for plugin", "Empty plugin",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q in output:\n%s", want, output)
		}
	}
}

func TestValidateReportsSyntaxAndRecoveredBlocks(t *testing.T) {
	root := t.TempDir()
	put(t, root, "slopshop.hcl", `
marketplace {
  name = "slopshop"
  name = "duplicate"
  display_name = "Slopshop"
}
plugin "example" {
  version = "1.0.0"
  description = ""
}
`)
	output, err := executeCommand(root, "validate")
	if err == nil || !strings.Contains(output, "Attribute redefined") || !strings.Contains(output, "Empty description") {
		t.Fatalf("validate: %v\n%s", err, output)
	}
}

func TestValidateOnlyReadsManifest(t *testing.T) {
	root := t.TempDir()
	put(t, root, "slopshop.hcl", testManifest)
	put(t, root, "slopshop.lock.json", "invalid JSON")

	output, err := executeCommand(root, "validate", "--verbose")
	if err != nil || !strings.Contains(output, "slopshop.hcl is valid.") || !strings.Contains(output, "Reading slopshop.hcl") {
		t.Fatalf("validate: %v\n%s", err, output)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 2 {
		t.Fatalf("validate changed directory contents: %v, %v", entries, err)
	}
}

func TestValidateOutputFormats(t *testing.T) {
	root := fixture(t)
	invalid := strings.Replace(testManifest, "0.1.0", "v1.2.3", 1)
	put(t, root, "slopshop.hcl", invalid)

	rich, err := executeCommand(root, "validate")
	if err != errInvalidManifest || !strings.Contains(rich, "slopshop.hcl:7:12:") ||
		!strings.Contains(rich, `7 |  version = "v1.2.3"`) || !strings.Contains(rich, "^^^^^^^^") {
		t.Fatalf("rich: %v\n%s", err, rich)
	}
	plain, err := executeCommand(root, "validate", "-o", "plain")
	if err != errInvalidManifest || strings.Count(plain, "\n") != 1 ||
		!strings.Contains(plain, "slopshop.hcl:7:12: error: Invalid plugin version") {
		t.Fatalf("plain: %v\n%s", err, plain)
	}

	var stdout, stderr bytes.Buffer
	command := newCommand(root, &stdout, &stderr)
	command.SetArgs([]string{"validate", "--output", "json", "--verbose"})
	if err := command.Execute(); err != errInvalidManifest {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	var result validationResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Valid || len(result.Diagnostics) != 1 {
		t.Fatalf("invalid JSON result: %v\n%s", err, stdout.String())
	}
	diagnostic := result.Diagnostics[0]
	if diagnostic.Severity != "error" || diagnostic.Summary != "Invalid plugin version" ||
		diagnostic.Range == nil || diagnostic.Range.Start.Line != 7 || diagnostic.Range.Start.Column != 12 {
		t.Fatalf("unexpected diagnostic: %+v", diagnostic)
	}
	if !strings.Contains(stderr.String(), "Reading slopshop.hcl") {
		t.Fatalf("missing verbose progress on stderr: %s", stderr.String())
	}

	put(t, root, "slopshop.hcl", testManifest)
	output, err := executeCommand(root, "validate", "-o", "json")
	if err != nil || json.Unmarshal([]byte(output), &result) != nil || !result.Valid ||
		result.Diagnostics == nil || len(result.Diagnostics) != 0 {
		t.Fatalf("valid JSON result: %v\n%s", err, output)
	}
	_, err = executeCommand(root, "validate", "--output", "yaml")
	expectError(t, err, "invalid output format")
}

func TestValidatePluginVersion(t *testing.T) {
	for _, version := range []string{"0.0.0", "1.2.3-rc.1+build.005", "1.2.3-01alpha", "1.2.3+001"} {
		t.Run(version, func(t *testing.T) {
			root := t.TempDir()
			put(t, root, "slopshop.hcl", strings.Replace(testManifest, "0.1.0", version, 1))
			if _, err := executeCommand(root, "validate"); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, version := range []string{"v1.2.3", "1.2", "01.2.3", "1.2.3-01", "1.2.3-alpha..1", "1.2.3+", "1.2.3+build_1", "1.2.3 "} {
		t.Run(version, func(t *testing.T) {
			root := t.TempDir()
			put(t, root, "slopshop.hcl", strings.Replace(testManifest, "0.1.0", version, 1))
			output, err := executeCommand(root, "validate")
			if err == nil || !strings.Contains(output, "Invalid plugin version") {
				t.Fatalf("validate: %v\n%s", err, output)
			}
		})
	}
}

func TestMCPTransportValidation(t *testing.T) {
	for _, tc := range []struct{ name, fields, want string }{
		{"missing URL", `type = "http"`, `"url" attribute is required`},
		{"missing command", `type = "stdio"`, `"command" attribute is required`},
		{"invalid URL", "type = \"http\"\nurl = \"http://example.com/mcp\"", `"url" attribute must be`},
		{"invalid headers", "type = \"http\"\nurl = \"https://example.com/mcp\"\nheaders = { Accept = \"a\", accept = \"b\" }", `"headers" attribute must be`},
		{"invalid args", "type = \"stdio\"\ncommand = \"npx\"\nargs = [true]", `"args" attribute must be`},
		{"wrong transport", "type = \"http\"\nurl = \"https://example.com/mcp\"\ncommand = \"npx\"", `"command" attribute is not allowed`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			start, _, _ := strings.Cut(testManifest, `  type = "stdio"`)
			put(t, root, "slopshop.hcl", start+tc.fields+"\n }\n}\n")
			output, err := executeCommand(root, "validate", "-o", "plain")
			if err == nil || !strings.Contains(output, tc.want) {
				t.Fatalf("validate: %v\n%s", err, output)
			}
			for _, mode := range []string{"generate", "check"} {
				expectError(t, run(root, mode), tc.want)
			}
		})
	}
}

func TestGenerateHTTPServer(t *testing.T) {
	root := fixture(t)
	put(t, root, "slopshop.hcl", `
marketplace {
  name = "test"
  display_name = "Test"
}
plugin "firefox" {
  version = "0.1.0"
  mcp_server "foxmcp" {
    type = "http"
    url = "https://example.com/mcp"
  }
}
`)
	mustRun(t, root, "generate")
	data, err := os.ReadFile(filepath.Join(root, "plugins", "firefox", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"type": "streamable-http"`) || !strings.Contains(string(data), `"url": "https://example.com/mcp"`) {
		t.Fatalf("unexpected HTTP server configuration: %s", data)
	}
}

func TestGenerateWithoutPluginDescription(t *testing.T) {
	root := fixture(t)
	mustRun(t, root, "generate")
	put(t, root, "slopshop.hcl", strings.Replace(testManifest, " description = \"A plugin\"\n", "", 1))
	if _, err := executeCommand(root, "validate"); err != nil {
		t.Fatal(err)
	}
	mustRun(t, root, "generate")
	mustRun(t, root, "check")

	data, err := os.ReadFile(filepath.Join(root, "plugins", "firefox", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	if _, exists := metadata["description"]; exists {
		t.Fatalf("unexpected description in generated manifest: %s", data)
	}
}

func TestImportLifecycle(t *testing.T) {
	root := fixture(t)
	manifest := strings.Replace(testManifest, `skill "local-skill" { path = "local-skill" }`, "skill \"local-skill\" { path = \"local-skill\" }\n "+remoteSkillBlock, 1)
	put(t, root, "slopshop.hcl", manifest)
	fetchFixture := func(source, skillPath, ref, work string) (string, string, error) {
		p := filepath.Join(work, "source")
		if err := os.MkdirAll(p, 0755); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("---\nname: remote-skill\ndescription: Remote skill\n---\n\nContent\n"), 0644); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(filepath.Join(p, "reference.txt"), []byte("reference"), 0644); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(filepath.Join(p, "_SKILL.md"), []byte("upstream extra file"), 0644); err != nil {
			return "", "", err
		}
		return p, strings.Repeat("a", 40), nil
	}
	expectError(t, runWithFetcher(root, "check", fetchFixture), "out of date")
	if err := runWithFetcher(root, "generate", fetchFixture); err != nil {
		t.Fatal(err)
	}
	if err := runWithFetcher(root, "check", fetchFixture); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "plugins/firefox/skills/remote-skill/reference.txt")); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "plugins/firefox/skills/remote-skill/_SKILL.md")); err != nil || string(data) != "upstream extra file" {
		t.Fatalf("external extra file changed: %q %v", data, err)
	}
	put(t, root, "plugins/firefox/skills/remote-skill/reference.txt", "modified")
	expectError(t, run(root, "check"), "local changes")
	expectError(t, runWithFetcher(root, "generate", fetchFixture), "local changes")
	put(t, root, "plugins/firefox/skills/remote-skill/reference.txt", "reference")
	put(t, root, "slopshop.hcl", testManifest)
	expectError(t, run(root, "check"), "out of date")
	if err := runWithFetcher(root, "generate", fetchFixture); err != nil {
		t.Fatal(err)
	}
	mustRun(t, root, "check")
	if _, err := os.Stat(filepath.Join(root, "plugins/firefox/skills/remote-skill")); !os.IsNotExist(err) {
		t.Fatalf("removed import still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "plugins/firefox/skills/local-skill/SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestImportChangeReport(t *testing.T) {
	root := fixture(t)
	put(t, root, "slopshop.hcl", strings.Replace(testManifest, `skill "local-skill" { path = "local-skill" }`, "skill \"local-skill\" { path = \"local-skill\" }\n "+remoteSkillBlock, 1))
	var output bytes.Buffer
	body := "First version"
	fetch := func(source, skillPath, ref, work string) (string, string, error) {
		dir := filepath.Join(work, "source")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", "", err
		}
		err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: remote-skill\ndescription: Remote\n---\n"+body+"\n"), 0644)
		return dir, strings.Repeat("a", 40), err
	}
	if err := runWithReporter(root, "generate", fetch, newReporter(&output, true)); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Fetching firefox/remote-skill", "plugins/firefox/skills/remote-skill/SKILL.md", "slopshop.lock.json"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("missing %q in %s", text, output.String())
		}
	}
	output.Reset()
	body = "Second version"
	expectError(t, runWithReporter(root, "check", fetch, newReporter(&output, false)), "out of date")
	for _, text := range []string{"UPDATE plugins/firefox/skills/remote-skill/SKILL.md", "-First version", "+Second version", "ref v1", "$.skills.firefox/remote-skill.sha256"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("missing %q in %s", text, output.String())
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "plugins/firefox/skills/remote-skill/SKILL.md"))
	if err != nil || !strings.Contains(string(data), "First version") {
		t.Fatalf("check changed the installed skill: %q, %v", data, err)
	}
	if err := runWithFetcher(root, "generate", fetch); err != nil {
		t.Fatal(err)
	}
	if err := runWithFetcher(root, "check", fetch); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	put(t, root, "slopshop.hcl", testManifest)
	if err := runWithReporter(root, "generate", fetch, newReporter(&output, false)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "REMOVE plugins/firefox/skills/remote-skill/SKILL.md") || !strings.Contains(output.String(), "$.skills.firefox/remote-skill") {
		t.Fatalf("missing removal details: %s", output.String())
	}
}

func TestRejectUntrackedImportDestination(t *testing.T) {
	root := fixture(t)
	put(t, root, "plugins/firefox/skills/local-skill/SKILL.md", "untracked content")
	expectError(t, run(root, "generate"), "refusing to overwrite untracked")
}

func TestSourcePaths(t *testing.T) {
	for _, source := range []string{"https://git.example.com/team/repo.git", "git@example.com:team/repo.git", "example.com:team/repo.git", "ssh://git@example.com/team/repo.git"} {
		if err := validateRepositoryURL(source); err != nil {
			t.Errorf("rejected %s: %v", source, err)
		}
	}
	for _, source := range []string{"file:///repo", "http://example.com/repo", "-c", "https:///missing-host"} {
		if err := validateRepositoryURL(source); err == nil {
			t.Errorf("accepted %s", source)
		}
	}
	for _, path := range []string{"../secret", "/absolute", "a/../../b", "C:/other", "a\\b"} {
		if err := validateSkillPath(path); err == nil {
			t.Errorf("accepted %s", path)
		}
	}
	for _, path := range []string{".", "skills/my-skill"} {
		if err := validateSkillPath(path); err != nil {
			t.Errorf("rejected %s: %v", path, err)
		}
	}
}

func TestFetchSkillFromGit(t *testing.T) {
	upstream := t.TempDir()
	if _, err := git("init", "--quiet", upstream); err != nil {
		t.Fatal(err)
	}
	put(t, upstream, "skills/remote-skill/SKILL.md", "---\nname: remote-skill\ndescription: Remote skill\n---\n\nContent\n")
	if _, err := git("-C", upstream, "add", "."); err != nil {
		t.Fatal(err)
	}
	if _, err := git("-C", upstream, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--quiet", "-m", "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := git("-C", upstream, "tag", "v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := git("-C", upstream, "branch", "feature/example"); err != nil {
		t.Fatal(err)
	}
	commit, err := git("-C", upstream, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url."+filepath.ToSlash(upstream)+".insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://github.com/example/repo")
	dir, resolved, err := fetchSkill("https://github.com/example/repo", "skills/remote-skill", "v1", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if resolved != commit {
		t.Fatalf("resolved %q, want %q", resolved, commit)
	}
	if err := validateSkillFile(dir, "remote-skill"); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"", commit, "feature/example"} {
		_, resolved, err := fetchSkill("https://github.com/example/repo", "skills/remote-skill", ref, t.TempDir())
		if err != nil || resolved != commit {
			t.Fatalf("ref %q: %s %v", ref, resolved, err)
		}
	}
	root := fixture(t)
	withoutRef := strings.Replace(remoteSkillBlock, "\n  ref = \"v1\"", "", 1)
	put(t, root, "slopshop.hcl", strings.Replace(testManifest, `skill "local-skill" { path = "local-skill" }`, `skill "local-skill" { path = "local-skill" }`+"\n"+withoutRef, 1))
	mustRun(t, root, "generate")
	lock, err := readLock(root)
	if err != nil {
		t.Fatal(err)
	}
	entry := lock.Skills["firefox/remote-skill"]
	if entry.Ref != "" || entry.Path != "skills/remote-skill" || entry.Commit != commit {
		t.Fatalf("unexpected lock entry: %+v", entry)
	}
	mustRun(t, root, "check")

	if _, err := git("-C", upstream, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--quiet", "--allow-empty", "-m", "advance HEAD"); err != nil {
		t.Fatal(err)
	}
	nextCommit, err := git("-C", upstream, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	output, err := executeCommand(root, "check")
	if err == nil || !strings.Contains(output, "$.skills.firefox/remote-skill.commit") || !strings.Contains(output, nextCommit) {
		t.Fatalf("check missed the new upstream commit: %v\n%s", err, output)
	}
	lock, err = readLock(root)
	if err != nil || lock.Skills["firefox/remote-skill"] != entry {
		t.Fatalf("check changed the lock: %+v, %v", lock, err)
	}

	mustRun(t, root, "generate")
	lock, err = readLock(root)
	if err != nil || lock.Skills["firefox/remote-skill"].Commit != nextCommit ||
		lock.Skills["firefox/remote-skill"].Hash != entry.Hash {
		t.Fatalf("generate did not refresh the commit: %+v, %v", lock, err)
	}
	mustRun(t, root, "check")
}
