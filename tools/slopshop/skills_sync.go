package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type stagedImport struct {
	key    string
	path   string
	target string
	entry  importEntry
}

func syncSkills(root string, manifest *manifest, old lockFile, mode string, fetch skillFetcher, report *reporter) (lockFile, error) {
	if err := validateSourceFilenames(root); err != nil {
		return old, err
	}
	if err := validateSkillEntriesForSync(root, manifest, old); err != nil {
		return old, err
	}
	if err := verifyExistingImports(root, manifest, old); err != nil {
		return old, err
	}

	workRoot := root
	if mode == "check" {
		workRoot = ""
	}
	workDir, err := os.MkdirTemp(workRoot, ".slopshop-sync-")
	if err != nil {
		return old, err
	}
	keepWorkDir := false
	defer func() {
		if !keepWorkDir {
			_ = os.RemoveAll(workDir)
		}
	}()

	staged, err := stageImports(root, workDir, manifest, old, fetch, report)
	if err != nil {
		return old, err
	}
	next := lockFile{Skills: make(map[string]importEntry)}
	for _, item := range staged {
		next.Skills[item.key] = item.entry
	}
	previousChanges := report.changes
	if err := reportImports(root, staged, old, report); err != nil {
		return old, err
	}
	if report.changes == previousChanges {
		report.progress("Skills and lock are unchanged")
		return next, nil
	}
	if mode == "check" {
		return next, nil
	}
	err = installImports(root, workDir, staged, old)
	var recovery *recoveryError
	if errors.As(err, &recovery) {
		keepWorkDir = true
	}
	return next, err
}

// Verify every owned directory, including those that generation will remove.
func verifyExistingImports(root string, manifest *manifest, lock lockFile) error {
	local := make(map[string]bool)
	for _, plugin := range manifest.Plugins {
		for _, skill := range plugin.Skills {
			if skill.Source == "" {
				local[plugin.Name+"/"+skill.Name] = true
			}
		}
	}
	for key, entry := range lock.Skills {
		dir := importedSkillPath(root, key)
		if _, err := os.Lstat(dir); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		hash, err := hashDir(dir)
		if err != nil {
			return fmt.Errorf("import %s: %w", key, err)
		}
		if hash != entry.Hash && !local[key] {
			return fmt.Errorf("import %s has local changes; refusing to overwrite or remove", key)
		}
	}
	return nil
}

func stageImports(root, workDir string, manifest *manifest, old lockFile, fetch skillFetcher, report *reporter) ([]stagedImport, error) {
	var staged []stagedImport
	for _, plugin := range manifest.Plugins {
		for _, skill := range plugin.Skills {
			key := plugin.Name + "/" + skill.Name
			target := importedSkillPath(root, key)
			_, tracked := old.Skills[key]
			item, err := stageImport(root, workDir, key, target, skill, len(staged), fetch, report)
			if err != nil {
				return nil, err
			}
			if !tracked {
				if _, err := os.Lstat(target); err == nil {
					hash, err := hashDir(target)
					if err != nil {
						return nil, err
					}
					if hash != item.entry.Hash {
						return nil, fmt.Errorf("refusing to overwrite untracked skill %s", key)
					}
				} else if !os.IsNotExist(err) {
					return nil, err
				}
			}
			staged = append(staged, item)
		}
	}
	return staged, nil
}

// Compare staged skill files and lock entries before installing them.
func reportImports(root string, staged []stagedImport, old lockFile, report *reporter) error {
	next := lockFile{Skills: make(map[string]importEntry)}
	byKey := make(map[string]stagedImport)
	for _, item := range staged {
		next.Skills[item.key] = item.entry
		byKey[item.key] = item
	}
	keys := make(map[string]bool)
	for key := range old.Skills {
		keys[key] = true
	}
	for key := range next.Skills {
		keys[key] = true
	}
	names := make([]string, 0, len(keys))
	for key := range keys {
		names = append(names, key)
	}
	sort.Strings(names)
	for _, key := range names {
		item, exists := byKey[key]
		previousChanges := report.changes
		oldFiles, err := skillFiles(importedSkillPath(root, key))
		if err != nil {
			return err
		}
		newFiles := map[string][]byte{}
		if exists {
			newFiles, err = skillFiles(item.path)
			if err != nil {
				return err
			}
		}
		files := make(map[string]bool)
		for name := range oldFiles {
			files[name] = true
		}
		for name := range newFiles {
			files[name] = true
		}
		fileNames := make([]string, 0, len(files))
		for name := range files {
			fileNames = append(fileNames, name)
		}
		sort.Strings(fileNames)
		for _, name := range fileNames {
			before, was := oldFiles[name]
			after, now := newFiles[name]
			if was && now && bytes.Equal(before, after) {
				continue
			}
			action, reason := "UPDATE", skillChangeReason(item.entry)
			if !was {
				action = "CREATE"
			}
			if !now {
				action = "REMOVE"
				reason = "This file is no longer present in the imported skill or the skill was removed from the manifest."
			}
			parts := strings.Split(key, "/")
			var details []string
			if was && now {
				details = report.textDifferences(before, after)
			}
			report.change(action, filepath.Join("plugins", parts[0], "skills", parts[1], name), reason, details)
		}
		if exists && report.changes == previousChanges {
			if hash, err := hashDir(importedSkillPath(root, key)); err == nil && hash != item.entry.Hash {
				parts := strings.Split(key, "/")
				report.change("UPDATE", filepath.Join("plugins", parts[0], "skills", parts[1]), skillChangeReason(item.entry), []string{"Directory structure changed."})
			}
		}
	}
	lockPath := filepath.Join(root, "slopshop.lock.json")
	before, err := os.ReadFile(lockPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	missing := os.IsNotExist(err)
	after, err := jsonBytes(next)
	if err != nil {
		return err
	}
	if !bytes.Equal(before, after) {
		if missing {
			report.change("CREATE", "slopshop.lock.json", "Record skill source paths, content hashes and resolved commits.", nil)
		} else {
			report.change("UPDATE", "slopshop.lock.json", "Record skill source paths, content hashes and resolved commits.", report.jsonDifferences(before, after))
		}
	}
	return nil
}

func skillChangeReason(entry importEntry) string {
	if entry.Source == "" {
		return fmt.Sprintf("Copied from skills-source/%s.", entry.Path)
	}
	ref := entry.Ref
	if ref == "" {
		ref = "HEAD"
	}
	return fmt.Sprintf("Copied from %s at %s (ref %s, commit %s).", entry.Source, entry.Path, ref, entry.Commit)
}

func skillFiles(dir string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return files, nil
	} else if err != nil {
		return nil, err
	}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err == nil {
			files[rel] = data
		}
		return err
	})
	return files, err
}

func stageImport(root, workDir, key, target string, skill skill, index int, fetch skillFetcher, report *reporter) (stagedImport, error) {
	work := filepath.Join(workDir, fmt.Sprint(index))
	if err := os.MkdirAll(work, 0755); err != nil {
		return stagedImport{}, err
	}
	var source, commit string
	var err error
	if skill.Source == "" {
		report.progress("Reading local skill %s from skills-source/%s", key, skill.Path)
		source, err = resolveSkillPath(filepath.Join(root, "skills-source"), skill.Path)
	} else {
		ref := skill.Ref
		if ref == "" {
			ref = "HEAD"
		}
		report.progress("Fetching %s from %s at %s", key, skill.Source, ref)
		source, commit, err = fetch(skill.Source, skill.Path, skill.Ref, work)
		if err == nil {
			report.progress("Resolved %s to %s", key, commit)
		}
	}
	if err != nil {
		return stagedImport{}, fmt.Errorf("skill %s: %w", key, err)
	}
	stagingPath := filepath.Join(work, "skill")
	if err := copyDir(source, stagingPath); err != nil {
		return stagedImport{}, err
	}
	if skill.Source == "" {
		if err := os.Rename(filepath.Join(stagingPath, "_SKILL.md"), filepath.Join(stagingPath, "SKILL.md")); err != nil {
			return stagedImport{}, fmt.Errorf("skill %s: expected _SKILL.md in local source: %w", key, err)
		}
	}
	if err := validateSkillFile(stagingPath, skill.Name); err != nil {
		return stagedImport{}, fmt.Errorf("import %s: %w", key, err)
	}
	hash, err := hashDir(stagingPath)
	if err != nil {
		return stagedImport{}, err
	}
	return stagedImport{
		key: key, path: stagingPath, target: target,
		entry: importEntry{Source: skill.Source, Path: skill.Path, Ref: skill.Ref, Commit: commit, Hash: hash},
	}, nil
}

type recoveryError struct {
	workDir string
	err     error
}

func (e *recoveryError) Error() string {
	return fmt.Sprintf("%v; rollback failed, backups retained at %s", e.err, e.workDir)
}

func (e *recoveryError) Unwrap() error { return e.err }

func installImports(root, workDir string, staged []stagedImport, old lockFile) error {
	lock := lockFile{Skills: make(map[string]importEntry)}
	for _, item := range staged {
		lock.Skills[item.key] = item.entry
	}
	lockTemp := filepath.Join(workDir, "new-lock.json")
	if err := writeJSON(lockTemp, lock); err != nil {
		return err
	}

	// Keep backups until both the directories and their lock file are in place.
	backups := make(map[string]string)
	var installed []string
	rollback := func(cause error) error {
		var failures []error
		for _, target := range installed {
			if err := os.RemoveAll(target); err != nil {
				failures = append(failures, err)
			}
		}
		for target, backup := range backups {
			if err := os.Rename(backup, target); err != nil {
				failures = append(failures, err)
			}
		}
		if len(failures) > 0 {
			return &recoveryError{workDir: workDir, err: errors.Join(append([]error{cause}, failures...)...)}
		}
		return cause
	}

	keys := make([]string, 0, len(old.Skills))
	for key := range old.Skills {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for index, key := range keys {
		target := importedSkillPath(root, key)
		if _, err := os.Lstat(target); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return rollback(err)
		}
		backup := filepath.Join(workDir, fmt.Sprintf("backup-%d", index))
		if err := os.Rename(target, backup); err != nil {
			return rollback(err)
		}
		backups[target] = backup
	}

	for _, item := range staged {
		if err := os.MkdirAll(filepath.Dir(item.target), 0755); err != nil {
			return rollback(err)
		}
		// An untracked destination can be adopted only when it already matches.
		if _, tracked := old.Skills[item.key]; !tracked {
			if _, err := os.Lstat(item.target); err == nil {
				continue
			} else if !os.IsNotExist(err) {
				return rollback(err)
			}
		}
		if err := os.Rename(item.path, item.target); err != nil {
			return rollback(err)
		}
		installed = append(installed, item.target)
	}
	if err := os.Rename(lockTemp, filepath.Join(root, "slopshop.lock.json")); err != nil {
		return rollback(err)
	}
	return nil
}

func importedSkillPath(root, key string) string {
	parts := strings.Split(key, "/")
	return filepath.Join(root, "plugins", parts[0], "skills", parts[1])
}
