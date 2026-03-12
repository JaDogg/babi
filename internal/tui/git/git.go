package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// FileStatus represents a single changed file from git status.
type FileStatus struct {
	Path string
	X, Y byte // X=staging area, Y=working tree
}

// Label returns the 2-char XY status string.
func (f FileStatus) Label() string {
	return string([]byte{f.X, f.Y})
}

// FindRoot returns the git repository root for the given directory.
func FindRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// GetStatus returns changed/untracked files in the repo.
func GetStatus(repoDir string) ([]FileStatus, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []FileStatus
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		x := line[0]
		y := line[1]
		path := strings.TrimSpace(line[3:])
		// Handle renames: "old -> new"
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		path = strings.Trim(path, `"`)
		files = append(files, FileStatus{Path: path, X: x, Y: y})
	}
	return files, nil
}

// StageFiles runs `git add -- <paths>`.
func StageFiles(repoDir string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	return cmd.Run()
}

// CommitWithMessage runs `git commit -m message`.
func CommitWithMessage(repoDir, message string) (string, error) {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// DisplayItem is a row in the file list view.
type DisplayItem struct {
	IsDir    bool
	Path     string // relative path (file) or dir prefix (dir header)
	Name     string // display name
	Status   string // e.g. "M ", " M", "??"
	Depth    int
	Selected bool
}

// BuildItems creates a grouped display list from git status files.
func BuildItems(files []FileStatus) []DisplayItem {
	type group struct {
		dir   string
		files []FileStatus
	}
	seen := make(map[string]int)
	var groups []group
	for _, f := range files {
		dir := filepath.Dir(f.Path)
		if dir == "." {
			dir = ""
		}
		if idx, ok := seen[dir]; ok {
			groups[idx].files = append(groups[idx].files, f)
		} else {
			seen[dir] = len(groups)
			groups = append(groups, group{dir: dir, files: []FileStatus{f}})
		}
	}
	// Sort: root first, then alphabetical
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].dir == "" {
			return true
		}
		if groups[j].dir == "" {
			return false
		}
		return groups[i].dir < groups[j].dir
	})

	var items []DisplayItem
	for _, g := range groups {
		if g.dir != "" {
			items = append(items, DisplayItem{
				IsDir: true,
				Path:  g.dir,
				Name:  g.dir + "/",
			})
		}
		for _, f := range g.files {
			depth := 0
			if g.dir != "" {
				depth = 1
			}
			items = append(items, DisplayItem{
				IsDir:    false,
				Path:     f.Path,
				Name:     filepath.Base(f.Path),
				Status:   f.Label(),
				Depth:    depth,
				Selected: true,
			})
		}
	}
	return items
}

// SelectedPaths returns all selected file paths.
func SelectedPaths(items []DisplayItem) []string {
	var paths []string
	for _, it := range items {
		if !it.IsDir && it.Selected {
			paths = append(paths, it.Path)
		}
	}
	return paths
}

// ToggleDir toggles all files whose directory matches dirPath (recursively).
func ToggleDir(items []DisplayItem, dirPath string) []DisplayItem {
	allSelected := true
	found := false
	for _, it := range items {
		if !it.IsDir && matchesDir(it.Path, dirPath) {
			found = true
			if !it.Selected {
				allSelected = false
			}
		}
	}
	if !found {
		return items
	}
	newState := !allSelected
	result := make([]DisplayItem, len(items))
	copy(result, items)
	for i, it := range result {
		if !it.IsDir && matchesDir(it.Path, dirPath) {
			result[i].Selected = newState
		}
	}
	return result
}

func matchesDir(filePath, dirPath string) bool {
	dir := filepath.Dir(filePath)
	if dir == "." {
		dir = ""
	}
	if dirPath == "" {
		return dir == ""
	}
	return dir == dirPath || strings.HasPrefix(dir, dirPath+"/")
}

// DirSelected returns 0=none, 1=some, 2=all selected for files under dirPath.
func DirSelected(items []DisplayItem, dirPath string) int {
	total, selected := 0, 0
	for _, it := range items {
		if !it.IsDir && matchesDir(it.Path, dirPath) {
			total++
			if it.Selected {
				selected++
			}
		}
	}
	if total == 0 || selected == 0 {
		return 0
	}
	if selected == total {
		return 2
	}
	return 1
}

// CommitInfo holds a single git log entry.
type CommitInfo struct {
	Hash    string // full hash
	Short   string // short hash (7 chars)
	RelTime string // relative time e.g. "2 hours ago"
	Author  string
	Subject string
}

// GetLog returns up to limit commits. limit <= 0 means 200.
func GetLog(repoDir string, limit int) ([]CommitInfo, error) {
	if limit <= 0 {
		limit = 200
	}
	cmd := exec.Command("git", "log",
		fmt.Sprintf("-n%d", limit),
		"--pretty=format:%H\x1f%h\x1f%ar\x1f%an\x1f%s",
	)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var commits []CommitInfo
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) < 5 {
			continue
		}
		commits = append(commits, CommitInfo{
			Hash:    parts[0],
			Short:   parts[1],
			RelTime: parts[2],
			Author:  parts[3],
			Subject: parts[4],
		})
	}
	return commits, nil
}

// ShowCommit returns the full diff output for a commit hash.
func ShowCommit(repoDir, hash string) (string, error) {
	cmd := exec.Command("git", "show", "--stat", "--patch", hash)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	return string(out), err
}

// StashInfo holds a single stash entry.
type StashInfo struct {
	Ref     string // e.g. "stash@{0}"
	Index   int
	RelTime string
	Message string
}

// GetStashes returns all stash entries.
func GetStashes(repoDir string) ([]StashInfo, error) {
	cmd := exec.Command("git", "stash", "list", "--pretty=format:%gd\x1f%ar\x1f%s")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var stashes []StashInfo
	for i, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		ref := fmt.Sprintf("stash@{%d}", i)
		relTime, msg := "", ""
		if len(parts) >= 1 {
			ref = parts[0]
		}
		if len(parts) >= 2 {
			relTime = parts[1]
		}
		if len(parts) >= 3 {
			msg = strings.Join(parts[2:], " ")
		}
		stashes = append(stashes, StashInfo{Ref: ref, Index: i, RelTime: relTime, Message: msg})
	}
	return stashes, nil
}

// ShowStash returns the patch diff for a stash ref.
func ShowStash(repoDir, ref string) (string, error) {
	cmd := exec.Command("git", "stash", "show", "-p", ref)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	return string(out), err
}

// ApplyStash applies (keeps) a stash. Returns combined output.
func ApplyStash(repoDir, ref string) (string, error) {
	cmd := exec.Command("git", "stash", "apply", ref)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// PopStash applies and removes a stash. Returns combined output.
func PopStash(repoDir, ref string) (string, error) {
	cmd := exec.Command("git", "stash", "pop", ref)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// DropStash deletes a stash. Returns combined output.
func DropStash(repoDir, ref string) (string, error) {
	cmd := exec.Command("git", "stash", "drop", ref)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// CreateStash saves current changes to a new stash with optional message.
func CreateStash(repoDir, message string) (string, error) {
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "-m", message)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
