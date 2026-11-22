package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type importEntry struct {
	Source string `json:"source,omitempty"`
	Path   string `json:"path"`
	Ref    string `json:"ref,omitempty"`
	Commit string `json:"commit,omitempty"`
	Hash   string `json:"sha256"`
}
type lockFile struct {
	Skills map[string]importEntry `json:"skills"`
}

var (
	sshSource   = regexp.MustCompile(`^(?:[^@\s/:]+@)?[^@\s/:]+:[^\s]+$`)
	commitHash  = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
	contentHash = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

func validateRepositoryURL(source string) error {
	if strings.ContainsAny(source, "\r\n\t ") {
		return fmt.Errorf("invalid repository URL %q", source)
	}
	if !strings.Contains(source, "://") && !strings.Contains(source, `\`) && !strings.HasPrefix(source, "-") && sshSource.MatchString(source) {
		return nil
	}
	u, err := url.Parse(source)
	if err != nil || (u.Scheme != "https" && u.Scheme != "ssh") || u.Host == "" || u.Path == "" {
		return fmt.Errorf("source must be an HTTPS or Git SSH repository URL")
	}
	return nil
}

func validateSkillPath(skillPath string) error {
	if skillPath == "" || strings.ContainsAny(skillPath, "\\:\x00") || strings.HasPrefix(skillPath, "/") || path.Clean(skillPath) != skillPath || skillPath == ".." || strings.HasPrefix(skillPath, "../") {
		return fmt.Errorf("invalid skill path %q", skillPath)
	}
	return nil
}

func readLock(root string) (lockFile, error) {
	lock := lockFile{Skills: map[string]importEntry{}}
	data, err := os.ReadFile(filepath.Join(root, "slopshop.lock.json"))
	if os.IsNotExist(err) {
		return lock, nil
	}
	if err != nil {
		return lock, err
	}
	if err = json.Unmarshal(data, &lock); err != nil {
		return lock, fmt.Errorf("lock: %w", err)
	}
	if lock.Skills == nil {
		return lock, fmt.Errorf("lock: missing skills")
	}
	for key, entry := range lock.Skills {
		parts := strings.Split(key, "/")
		validKey := len(parts) == 2 && validName.MatchString(parts[0]) && validName.MatchString(parts[1])
		validCommit := entry.Source == "" && entry.Commit == "" || entry.Source != "" && commitHash.MatchString(entry.Commit)
		if !validKey || !validCommit || !contentHash.MatchString(entry.Hash) || validateSkillPath(entry.Path) != nil {
			return lock, fmt.Errorf("invalid lock entry %q", key)
		}
	}
	return lock, nil
}

func hashDir(dir string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(dir, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if filePath == dir {
			return nil
		}
		relativePath, err := filepath.Rel(dir, filePath)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in skill: %s", filePath)
		}
		if entry.IsDir() {
			fmt.Fprintf(hash, "d %s\n", filepath.ToSlash(relativePath))
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported file: %s", filePath)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(hash, "f %s %x\n", filepath.ToSlash(relativePath), sum)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(src, filePath)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, relativePath)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in imported skill: %s", filePath)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported file: %s", filePath)
		}
		in, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer in.Close()
		info, err := entry.Info()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

type skillFetcher func(source, skillPath, ref, work string) (string, string, error)

// fetchSkill returns a directory inside work and the resolved commit.
func fetchSkill(source, skillPath, ref, work string) (string, string, error) {
	if err := validateRepositoryURL(source); err != nil {
		return "", "", err
	}
	if err := validateSkillPath(skillPath); err != nil {
		return "", "", err
	}
	clone := filepath.Join(work, "repo")
	if _, err := git("clone", "--quiet", "--no-checkout", "-c", "core.autocrlf=false", "--", source, clone); err != nil {
		return "", "", err
	}
	if ref == "" {
		ref = "HEAD"
	}
	commit, err := git("-C", clone, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		commit, err = git("-C", clone, "rev-parse", "--verify", "--end-of-options", "refs/remotes/origin/"+ref+"^{commit}")
		if err != nil {
			return "", "", err
		}
	}
	if _, err = git("-C", clone, "checkout", "--quiet", "--detach", commit); err != nil {
		return "", "", err
	}
	dir, err := resolveSkillPath(clone, skillPath)
	return dir, commit, err
}

// Reject symlinks in every component, including the source root.
func resolveSkillPath(root, skillPath string) (string, error) {
	if err := validateSkillPath(skillPath); err != nil {
		return "", err
	}
	dir := root
	components := []string{""}
	if skillPath != "." {
		components = append(components, strings.Split(skillPath, "/")...)
	}
	for _, component := range components {
		dir = filepath.Join(dir, component)
		info, e := os.Lstat(dir)
		if e != nil {
			return "", e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("source is not a real directory: %s", dir)
		}
	}
	return dir, nil
}
