package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func run(root, mode string) error {
	return runWithFetcher(root, mode, fetchSkill)
}

func runWithFetcher(root, mode string, fetch skillFetcher) error {
	return runWithReporter(root, mode, fetch, newReporter(io.Discard, false))
}

func runWithReporter(root, mode string, fetch skillFetcher, report *reporter) error {
	report.progress("Reading slopshop.hcl")
	manifest, err := loadManifest(root)
	if err != nil {
		return err
	}
	report.progress("Reading slopshop.lock.json")
	lock, err := readLock(root)
	if err != nil {
		return err
	}

	report.progress("Comparing skills with their sources and the lock")
	lock, err = syncSkills(root, manifest, lock, mode, fetch, report)
	if err != nil {
		return err
	}
	report.progress("Comparing generated files")
	outputs := buildOutputs(manifest, lock)
	// Skill planning already reports lock changes, including in check mode.
	if mode == "check" {
		delete(outputs, "slopshop.lock.json")
	}
	planned, err := planOutputs(root, manifest, outputs, report)
	if err != nil {
		return err
	}
	if mode == "check" && report.changes > 0 {
		report.summary(mode)
		return fmt.Errorf("generated files are out of date; run generate")
	}
	for _, change := range planned {
		path := filepath.Join(root, change.name)
		if change.remove {
			if err := os.Remove(path); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, change.content, 0644); err != nil {
			return err
		}
	}
	report.summary(mode)
	return nil
}

func buildOutputs(manifest *manifest, lock lockFile) map[string]any {
	outputs := make(map[string]any)
	plugins := make([]any, 0, len(manifest.Plugins))
	skillPaths := []string{}

	for _, plugin := range manifest.Plugins {
		pluginDir := filepath.Join("plugins", plugin.Name)
		plugins = append(plugins, map[string]any{
			"name": plugin.Name,
			"source": map[string]any{
				"source": "local",
				"path":   "./plugins/" + plugin.Name,
			},
			"policy": map[string]any{
				"installation":   "AVAILABLE",
				"authentication": "ON_INSTALL",
			},
		})
		metadata := map[string]any{
			"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
			"name":    plugin.Name,
			"version": plugin.Version,
		}
		if plugin.Description != "" {
			metadata["description"] = plugin.Description
		}
		outputs[filepath.Join(pluginDir, "plugin.json")] = metadata

		if len(plugin.Servers) > 0 {
			servers := make(map[string]any, len(plugin.Servers))
			for _, server := range plugin.Servers {
				servers[server.Name] = server.Values
			}
			outputs[filepath.Join(pluginDir, "mcp.json")] = map[string]any{
				"$schema":    "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
				"mcpServers": servers,
			}
		}
		if len(plugin.Skills) > 0 {
			skillPaths = append(skillPaths, "./plugins/"+plugin.Name+"/skills")
		}
	}

	outputs[filepath.Join(".agents", "plugins", "marketplace.json")] = map[string]any{
		"name":      manifest.Marketplace.Name,
		"interface": map[string]any{"displayName": manifest.Marketplace.DisplayName},
		"plugins":   plugins,
	}
	outputs["skills.sh.json"] = map[string]any{
		"$schema": "https://skills.sh/schemas/skills.sh.schema.json",
		"paths":   skillPaths,
	}
	outputs["slopshop.lock.json"] = lock
	return outputs
}

type outputChange struct {
	name    string
	content []byte
	remove  bool
}

func planOutputs(root string, manifest *manifest, outputs map[string]any, report *reporter) ([]outputChange, error) {
	var changes []outputChange
	obsolete, err := removedPluginOutputs(root, manifest)
	if err != nil {
		return nil, err
	}
	for _, name := range obsolete {
		report.change("REMOVE", name, "The previous marketplace index lists a plugin removed from slopshop.hcl.", nil)
		changes = append(changes, outputChange{name: name, remove: true})
	}
	for _, plugin := range manifest.Plugins {
		if len(plugin.Servers) != 0 {
			continue
		}
		name := filepath.Join("plugins", plugin.Name, "mcp.json")
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		report.change("REMOVE", name, "The plugin has no mcp_server blocks in slopshop.hcl.", nil)
		changes = append(changes, outputChange{name: name, remove: true})
	}
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		expected, err := jsonBytes(outputs[name])
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", name, err)
		}
		path := filepath.Join(root, name)
		actual, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err == nil && bytes.Equal(actual, expected) {
			report.progress("Unchanged: %s", filepath.ToSlash(name))
			continue
		}
		reason := outputReason(name)
		if os.IsNotExist(err) {
			report.change("CREATE", name, reason, report.jsonDifferences([]byte("{}"), expected))
		} else {
			report.change("UPDATE", name, reason, report.jsonDifferences(actual, expected))
		}
		changes = append(changes, outputChange{name: name, content: expected})
	}
	return changes, nil
}

func removedPluginOutputs(root string, manifest *manifest) ([]string, error) {
	indexPath := filepath.Join(root, ".agents", "plugins", "marketplace.json")
	data, err := os.ReadFile(indexPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var index struct {
		Plugins []struct {
			Name   string
			Source struct {
				Source string
				Path   string
			}
		}
	}
	if json.Unmarshal(data, &index) != nil {
		// An invalid index provides no reliable ownership information.
		return nil, nil
	}
	declared := make(map[string]bool, len(manifest.Plugins))
	for _, plugin := range manifest.Plugins {
		declared[plugin.Name] = true
	}
	var obsolete []string
	for _, plugin := range index.Plugins {
		name := plugin.Name
		if declared[name] || !validName.MatchString(name) ||
			plugin.Source.Source != "local" || plugin.Source.Path != "./plugins/"+name {
			continue
		}
		// Ignore duplicate entries in the old index.
		declared[name] = true
		pluginDir, err := resolveSkillPath(filepath.Join(root, "plugins"), name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, file := range []string{"plugin.json", "mcp.json"} {
			info, err := os.Lstat(filepath.Join(pluginDir, file))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("obsolete plugin output is not a regular file: %s", info.Name())
			}
			obsolete = append(obsolete, filepath.Join("plugins", name, file))
		}
	}
	sort.Strings(obsolete)
	return obsolete, nil
}

func outputReason(name string) string {
	switch filepath.ToSlash(name) {
	case ".agents/plugins/marketplace.json":
		return "Generated from the marketplace and plugin blocks in slopshop.hcl."
	case "skills.sh.json":
		return "Lists plugin skill directories declared in slopshop.hcl."
	case "slopshop.lock.json":
		return "Records skill source paths, content hashes and resolved commits."
	}
	parts := strings.Split(filepath.ToSlash(name), "/")
	if len(parts) == 3 && parts[0] == "plugins" {
		switch parts[2] {
		case "plugin.json":
			return fmt.Sprintf("Generated from plugin %q metadata in slopshop.hcl.", parts[1])
		case "mcp.json":
			return fmt.Sprintf("Generated from plugin %q mcp_server blocks in slopshop.hcl.", parts[1])
		}
	}
	return "Generated from slopshop.hcl."
}

func jsonBytes(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeJSON(path string, value any) error {
	data, err := jsonBytes(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
