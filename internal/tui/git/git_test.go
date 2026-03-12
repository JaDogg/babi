package git

import (
	"testing"
)

func TestBuildItemsTree(t *testing.T) {
	files := []FileStatus{
		{Path: "main.go", X: 'M', Y: ' '},
		{Path: "internal/config/config.go", X: 'M', Y: ' '},
		{Path: "internal/tui/git/git.go", X: 'M', Y: ' '},
		{Path: "internal/tui/git/screen_files.go", X: ' ', Y: 'M'},
		{Path: "internal/tui/editor/app.go", X: '?', Y: '?'},
		{Path: "a/b/c/d/e/f/g/h/i/j/k/deep.go", X: 'M', Y: ' '},
	}

	items := BuildItems(files)

	// Build a lookup by path for convenient assertions.
	byPath := make(map[string]DisplayItem, len(items))
	for _, it := range items {
		byPath[it.Path] = it
	}

	cases := []struct {
		path    string
		isDir   bool
		depth   int
		wantName string
	}{
		// Root-level file
		{"main.go", false, 0, "main.go"},
		// Top-level dir header
		{"internal", true, 0, "internal/"},
		// Second-level dir headers
		{"internal/config", true, 1, "config/"},
		{"internal/tui", true, 1, "tui/"},
		// Third-level dir headers
		{"internal/tui/editor", true, 2, "editor/"},
		{"internal/tui/git", true, 2, "git/"},
		// Files at depth 3
		{"internal/config/config.go", false, 2, "config.go"},
		{"internal/tui/git/git.go", false, 3, "git.go"},
		{"internal/tui/git/screen_files.go", false, 3, "screen_files.go"},
		{"internal/tui/editor/app.go", false, 3, "app.go"},
		// 10-level-deep file: capped at depth 10
		{"a/b/c/d/e/f/g/h/i/j/k/deep.go", false, 10, "deep.go"},
	}

	for _, tc := range cases {
		it, ok := byPath[tc.path]
		if !ok {
			t.Errorf("path %q not found in items", tc.path)
			continue
		}
		if it.IsDir != tc.isDir {
			t.Errorf("path %q: IsDir=%v want %v", tc.path, it.IsDir, tc.isDir)
		}
		if it.Depth != tc.depth {
			t.Errorf("path %q: Depth=%d want %d", tc.path, it.Depth, tc.depth)
		}
		if it.Name != tc.wantName {
			t.Errorf("path %q: Name=%q want %q", tc.path, it.Name, tc.wantName)
		}
	}

	// Verify root-level files have no dir header emitted for them.
	for _, it := range items {
		if it.IsDir && it.Path == "" {
			t.Error("unexpected empty-path dir header for root")
		}
	}
}

func TestBuildItemsSelectedByDefault(t *testing.T) {
	files := []FileStatus{
		{Path: "foo.go", X: 'M', Y: ' '},
		{Path: "bar/baz.go", X: ' ', Y: 'M'},
	}
	items := BuildItems(files)
	for _, it := range items {
		if !it.IsDir && !it.Selected {
			t.Errorf("file %q should be selected by default", it.Path)
		}
	}
}

func TestBuildItemsOrder(t *testing.T) {
	// Dirs should appear before files at the same level; both sorted alpha.
	files := []FileStatus{
		{Path: "z.go", X: 'M', Y: ' '},
		{Path: "a.go", X: 'M', Y: ' '},
		{Path: "pkg/b.go", X: 'M', Y: ' '},
		{Path: "lib/c.go", X: 'M', Y: ' '},
	}
	items := BuildItems(files)

	// Expected flat order: lib/(dir), lib/c.go, pkg/(dir), pkg/b.go, a.go, z.go
	want := []string{"lib", "lib/c.go", "pkg", "pkg/b.go", "a.go", "z.go"}
	if len(items) != len(want) {
		t.Fatalf("got %d items, want %d", len(items), len(want))
	}
	for i, it := range items {
		if it.Path != want[i] {
			t.Errorf("items[%d].Path=%q want %q", i, it.Path, want[i])
		}
	}
}
