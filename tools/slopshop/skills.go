package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Reject names that the Skills CLI could discover anywhere in the source tree,
// including skills that are not currently declared in the manifest.
func validateSourceFilenames(root string) error {
	dir := filepath.Join(root, "skills-source")
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(dir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.EqualFold(entry.Name(), "SKILL.md") {
			return fmt.Errorf("source file %s can be discovered as a skill; rename it to _SKILL.md", name)
		}
		return nil
	})
}

func validateSkillFile(dir, name string) error {
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return fmt.Errorf("skill %s: %w", name, err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	parts := strings.SplitN(text, "\n---\n", 2)
	if !strings.HasPrefix(text, "---\n") || len(parts) != 2 {
		return fmt.Errorf("skill %s requires YAML frontmatter", name)
	}
	var frontmatter struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(strings.TrimPrefix(parts[0], "---\n")), &frontmatter); err != nil {
		return fmt.Errorf("skill %s: invalid frontmatter: %w", name, err)
	}
	if frontmatter.Name != name || strings.TrimSpace(frontmatter.Description) == "" {
		return fmt.Errorf("skill %s: invalid frontmatter, name or description", name)
	}
	return nil
}

func validateSkillEntriesForSync(root string, manifest *manifest, lock lockFile) error {
	pluginNames := make(map[string]bool)
	for _, plugin := range manifest.Plugins {
		pluginNames[plugin.Name] = true
	}
	for key := range lock.Skills {
		pluginNames[strings.Split(key, "/")[0]] = true
	}
	for name := range pluginNames {
		for _, dir := range []string{filepath.Join(root, "plugins"), filepath.Join(root, "plugins", name), filepath.Join(root, "plugins", name, "skills")} {
			info, err := os.Lstat(dir)
			if os.IsNotExist(err) {
				break
			}
			if err != nil {
				return err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("skill destination is not a real directory: %s", dir)
			}
		}
	}
	for _, plugin := range manifest.Plugins {
		dir := filepath.Join(root, "plugins", plugin.Name, "skills")
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", dir, err)
		}
		declared := make(map[string]bool)
		for _, skill := range plugin.Skills {
			declared[skill.Name] = true
		}
		for _, entry := range entries {
			_, tracked := lock.Skills[plugin.Name+"/"+entry.Name()]
			if !declared[entry.Name()] && !tracked {
				return fmt.Errorf("undeclared entry %s", filepath.Join(dir, entry.Name()))
			}
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("skill %s must be a real directory", entry.Name())
			}
		}
	}
	return nil
}
