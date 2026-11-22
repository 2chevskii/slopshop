package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

type manifest struct {
	Marketplace marketplace
	Plugins     []plugin
}

type marketplace struct {
	Name        string
	DisplayName string
}

type plugin struct {
	Name        string
	Version     string
	Description string
	Servers     []server
	Skills      []skill
}

type server struct {
	Name   string
	Values map[string]any
}

type skill struct {
	Name   string
	Source string
	Path   string
	Ref    string
}

// Version syntax from https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
const (
	versionNumber        = `(?:0|[1-9][0-9]*)`
	prereleaseIdentifier = `(?:` + versionNumber + `|[0-9]*[A-Za-z-][0-9A-Za-z-]*)`
	buildIdentifier      = `[0-9A-Za-z-]+`
)

var (
	validName    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	validVersion = regexp.MustCompile(`^` + versionNumber + `\.` + versionNumber + `\.` + versionNumber +
		`(?:-` + prereleaseIdentifier + `(?:\.` + prereleaseIdentifier + `)*)?` +
		`(?:\+` + buildIdentifier + `(?:\.` + buildIdentifier + `)*)?$`)

	manifestSchema = hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "marketplace"},
			{Type: "plugin", LabelNames: []string{"name"}},
		},
	}
	marketplaceSchema = hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "name", Required: true},
			{Name: "display_name", Required: true},
		},
	}
	pluginSchema = hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "version", Required: true},
			{Name: "description"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "mcp_server", LabelNames: []string{"name"}},
			{Type: "skill", LabelNames: []string{"name"}},
		},
	}
	skillSchema = hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "source"},
			{Name: "path", Required: true},
			{Name: "ref"},
		},
	}
)

func loadManifest(root string) (*manifest, error) {
	m, _, diagnostics := readManifest(root)
	if diagnostics.HasErrors() {
		var problems []error
		for _, diagnostic := range diagnostics {
			if diagnostic.Severity == hcl.DiagError {
				problems = append(problems, diagnostic)
			}
		}
		return nil, errors.Join(problems...)
	}
	return m, nil
}

func readManifest(root string) (*manifest, *hcl.File, hcl.Diagnostics) {
	parser := hclparse.NewParser()
	file, diagnostics := parser.ParseHCLFile(filepath.Join(root, "slopshop.hcl"))
	decoder := manifestDecoder{diagnostics: diagnostics}
	var m *manifest
	if file != nil {
		m = decoder.decode(file.Body.(*hclsyntax.Body))
	}
	diagnostics = decoder.diagnostics
	// HCL may return diagnostics in map iteration order. Report them in file order.
	sort.SliceStable(diagnostics, func(i, j int) bool {
		a, b := diagnostics[i], diagnostics[j]
		if a.Subject != nil && b.Subject != nil && a.Subject.Start.Byte != b.Subject.Start.Byte {
			return a.Subject.Start.Byte < b.Subject.Start.Byte
		}
		if (a.Subject == nil) != (b.Subject == nil) {
			return a.Subject == nil
		}
		return a.Error() < b.Error()
	})
	return m, file, diagnostics
}

type manifestDecoder struct {
	diagnostics hcl.Diagnostics
}

func (d *manifestDecoder) decode(body *hclsyntax.Body) *manifest {
	d.attributes(body, &manifestSchema)
	m := &manifest{}
	pluginNames := map[string]bool{}
	skillNames := map[string]bool{}
	var firstMarketplace *hclsyntax.Block

	// Inspect recognized blocks even when HCL reported an error in their labels.
	for _, block := range body.Blocks {
		switch block.Type {
		case "marketplace":
			market := d.decodeMarketplace(block.Body)
			if firstMarketplace == nil {
				firstMarketplace = block
				m.Marketplace = market
			} else {
				d.problem(block.TypeRange, "Duplicate marketplace block", "The first marketplace block is at %s.", firstMarketplace.TypeRange)
			}
		case "plugin":
			name := d.blockName(block, pluginNames)
			m.Plugins = append(m.Plugins, d.decodePlugin(block.Body, name, skillNames))
		}
	}
	if firstMarketplace == nil {
		d.problem(body.MissingItemRange(), "Missing marketplace block", "Exactly one marketplace block is required.")
	}
	sort.Slice(m.Plugins, func(i, j int) bool { return m.Plugins[i].Name < m.Plugins[j].Name })
	return m
}

func (d *manifestDecoder) decodeMarketplace(body *hclsyntax.Body) marketplace {
	attrs := d.attributes(body, &marketplaceSchema)
	name, ok := d.stringValue(attrs["name"])
	if ok && attrs["name"] != nil && !validName.MatchString(name) {
		d.problem(attrs["name"].Expr.Range(), "Invalid marketplace name", "The name %q must use lowercase letters and digits separated by single hyphens.", name)
	}
	return marketplace{Name: name, DisplayName: d.nonblankString(attrs["display_name"])}
}

func (d *manifestDecoder) decodePlugin(body *hclsyntax.Body, name string, skillNames map[string]bool) plugin {
	attrs := d.attributes(body, &pluginSchema)
	versionAttr := attrs["version"]
	version, ok := d.stringValue(versionAttr)
	if ok && versionAttr != nil && !validVersion.MatchString(version) {
		d.problem(versionAttr.Expr.Range(), "Invalid plugin version", "Version %q must follow Semantic Versioning 2.0.0, such as 1.2.3 or 1.2.3-rc.1+build.5.", version)
	}
	p := plugin{
		Name:        name,
		Version:     version,
		Description: d.nonblankString(attrs["description"]),
	}
	serverNames := map[string]bool{}
	for _, block := range body.Blocks {
		switch block.Type {
		case "mcp_server":
			name := d.blockName(block, serverNames)
			p.Servers = append(p.Servers, server{Name: name, Values: d.decodeServer(block.Body)})
		case "skill":
			name := d.blockName(block, skillNames)
			p.Skills = append(p.Skills, d.decodeSkill(block.Body, name))
		}
	}
	if len(p.Servers) == 0 && len(p.Skills) == 0 {
		d.problem(body.MissingItemRange(), "Empty plugin", "A plugin requires at least one mcp_server or skill block.")
	}
	sort.Slice(p.Skills, func(i, j int) bool { return p.Skills[i].Name < p.Skills[j].Name })
	return p
}

func (d *manifestDecoder) decodeSkill(body *hclsyntax.Body, name string) skill {
	attrs := d.attributes(body, &skillSchema)
	path, pathOK := d.stringValue(attrs["path"])
	source, sourceOK := d.stringValue(attrs["source"])
	ref, refOK := d.stringValue(attrs["ref"])
	if pathOK && attrs["path"] != nil {
		if err := validateSkillPath(path); err != nil {
			d.problem(attrs["path"].Expr.Range(), "Invalid skill path", "%s.", err)
		}
	}
	if sourceOK && source != "" {
		if err := validateRepositoryURL(source); err != nil {
			d.problem(attrs["source"].Expr.Range(), "Invalid skill source", "%s.", err)
		}
	}
	if refOK && ref != "" {
		if sourceOK && source == "" {
			d.problem(attrs["ref"].Expr.Range(), "Invalid skill ref", "A skill cannot set ref without source.")
		} else if ref != strings.TrimSpace(ref) || strings.HasPrefix(ref, "-") {
			d.problem(attrs["ref"].Expr.Range(), "Invalid skill ref", "A ref must have no leading or trailing whitespace and cannot start with a hyphen.")
		}
	}
	return skill{Name: name, Path: path, Source: source, Ref: ref}
}

func (d *manifestDecoder) decodeServer(body *hclsyntax.Body) map[string]any {
	for _, block := range body.Blocks {
		d.problem(block.TypeRange, "Unsupported MCP server block", "Use an attribute to configure %q. Nested blocks are not allowed.", block.Type)
	}
	if body.Attributes["type"] == nil {
		d.problem(body.MissingItemRange(), "Missing MCP server type", "Set type to \"stdio\" or \"http\".")
	}
	values := make(map[string]any, len(body.Attributes))
	for name, attr := range body.Attributes {
		value, diagnostics := attr.Expr.Value(nil)
		if diagnostics.HasErrors() {
			for _, diagnostic := range diagnostics {
				copy := *diagnostic
				copy.Detail = fmt.Sprintf("MCP server attributes accept static values only. %s", diagnostic.Detail)
				d.diagnostics = append(d.diagnostics, &copy)
			}
			continue
		}
		encoded, err := ctyjson.Marshal(value, value.Type())
		var decoded any
		if err == nil {
			err = json.Unmarshal(encoded, &decoded)
		}
		if err != nil {
			d.problem(attr.Expr.Range(), "Invalid MCP server attribute", "Attribute %q must be representable as JSON. %s", name, err)
			continue
		}
		if name == "type" {
			switch decoded {
			case "stdio":
			case "http":
				// Use the transport name required by the Agent Plugins schema.
				decoded = "streamable-http"
			default:
				d.problem(attr.Expr.Range(), "Invalid MCP server type", "Set type to \"stdio\" or \"http\".")
			}
		}
		values[name] = decoded
	}
	d.validateServerFields(body, values)
	return values
}

func (d *manifestDecoder) validateServerFields(body *hclsyntax.Body, values map[string]any) {
	transport := values["type"]
	var required string
	switch transport {
	case "stdio":
		required = "command"
	case "streamable-http":
		required = "url"
	default:
		return
	}
	if body.Attributes[required] == nil {
		d.problem(body.MissingItemRange(), "Missing MCP server attribute", "The %q attribute is required for this transport.", required)
	}
	for name, attr := range body.Attributes {
		if name == "type" {
			continue
		}
		var requirement string
		var valid bool
		value, evaluated := values[name]
		text, _ := value.(string)
		switch {
		case transport == "stdio" && name == "command":
			requirement = "a bare executable name or an executable path starting with ./"
			valid = validMCPCommand(text)
		case transport == "stdio" && name == "args":
			requirement = "a list of strings"
			valid = isStringList(value)
		case transport == "stdio" && name == "env":
			requirement = "a map of strings without PLUGIN_ROOT or PLUGIN_DATA"
			valid = isStringMap(value)
			if env, ok := value.(map[string]any); ok {
				for key := range env {
					valid = valid && key != "PLUGIN_ROOT" && key != "PLUGIN_DATA"
				}
			}
		case transport == "stdio" && name == "cwd":
			requirement = "a path rooted at ./, ${PLUGIN_ROOT}, or ${PLUGIN_DATA}"
			valid = validMCPCwd(text)
		case transport == "streamable-http" && name == "url":
			requirement = "an absolute HTTPS URL or loopback HTTP URL without user information or a fragment"
			valid = validMCPURL(text)
		case transport == "streamable-http" && name == "headers":
			requirement = "a map of valid HTTP headers with distinct names"
			valid = validMCPHeaders(value)
		default:
			d.problem(attr.NameRange, "Unsupported MCP server attribute", "The %q attribute is not allowed for this transport.", name)
			continue
		}
		if evaluated && !valid {
			d.problem(attr.Expr.Range(), "Invalid MCP server attribute", "The %q attribute must be %s.", name, requirement)
		}
	}
}

func validMCPCommand(command string) bool {
	if command == "" || command == "." || command == ".." || strings.Contains(command, "${") {
		return false
	}
	if relative, ok := strings.CutPrefix(command, "./"); ok {
		return path.Clean(relative) != "." && validMCPRelativePath(relative)
	}
	return !strings.ContainsAny(command, "/\\:\x00") && strings.IndexFunc(command, unicode.IsSpace) < 0
}

func validMCPCwd(cwd string) bool {
	for _, root := range []string{"./", "${PLUGIN_ROOT}", "${PLUGIN_DATA}"} {
		if root != "./" {
			if cwd == root {
				return true
			}
			root += "/"
		}
		if relative, ok := strings.CutPrefix(cwd, root); ok {
			return validMCPRelativePath(relative)
		}
	}
	return false
}

func validMCPRelativePath(relative string) bool {
	clean := path.Clean(relative)
	return !strings.ContainsAny(relative, "\\:\x00") && !path.IsAbs(relative) &&
		clean != ".." && !strings.HasPrefix(clean, "../")
}

func validMCPURL(value string) bool {
	endpoint, err := url.Parse(value)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || strings.Contains(value, "#") {
		return false
	}
	host := endpoint.Hostname()
	return endpoint.Scheme == "https" ||
		endpoint.Scheme == "http" && (strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback())
}

func isStringList(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}

func isStringMap(value any) bool {
	items, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}

var httpHeaderName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func validMCPHeaders(value any) bool {
	headers, ok := value.(map[string]any)
	if !ok {
		return false
	}
	names := make(map[string]bool, len(headers))
	for name, value := range headers {
		key := strings.ToLower(name)
		text, ok := value.(string)
		if !ok || !httpHeaderName.MatchString(name) || names[key] {
			return false
		}
		for i := range len(text) {
			if c := text[i]; c < ' ' && c != '\t' || c == 0x7f {
				return false
			}
		}
		names[key] = true
	}
	return true
}

func (d *manifestDecoder) attributes(body hcl.Body, schema *hcl.BodySchema) hcl.Attributes {
	content, diagnostics := body.Content(schema)
	d.diagnostics = append(d.diagnostics, diagnostics...)
	return content.Attributes
}

func (d *manifestDecoder) problem(subject hcl.Range, summary, format string, args ...any) {
	d.diagnostics = append(d.diagnostics, &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  summary,
		Detail:   fmt.Sprintf(format, args...),
		Subject:  &subject,
	})
}

func (d *manifestDecoder) blockName(block *hclsyntax.Block, names map[string]bool) string {
	if len(block.Labels) != 1 {
		return ""
	}
	name := block.Labels[0]
	switch {
	case !validName.MatchString(name):
		d.problem(block.LabelRanges[0], "Invalid "+block.Type+" name", "The name %q must use lowercase letters and digits separated by single hyphens.", name)
	case names[name]:
		d.problem(block.LabelRanges[0], "Duplicate "+block.Type, "The name %q is already used.", name)
	}
	names[name] = true
	return name
}

func (d *manifestDecoder) stringValue(attr *hcl.Attribute) (string, bool) {
	if attr == nil {
		return "", true
	}
	var value string
	diagnostics := gohcl.DecodeExpression(attr.Expr, nil, &value)
	d.diagnostics = append(d.diagnostics, diagnostics...)
	return value, !diagnostics.HasErrors()
}

func (d *manifestDecoder) nonblankString(attr *hcl.Attribute) string {
	value, ok := d.stringValue(attr)
	if ok && attr != nil && strings.TrimSpace(value) == "" {
		d.problem(attr.Expr.Range(), "Empty "+attr.Name, "The %s value must contain a non-whitespace character.", attr.Name)
	}
	return value
}
