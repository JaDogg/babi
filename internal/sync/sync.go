package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jadogg/babi/internal/config"
)

// ProgressMsg is sent during sync to report progress.
type ProgressMsg struct {
	EntryName string
	FilePath  string
	Done      bool
	Err       error
}

// SyncError records a per-file error.
type SyncError struct {
	Path string
	Err  error
}

// Result summarises the outcome of one SyncEntry.
type Result struct {
	Entry   config.SyncEntry
	Copied  int
	Skipped int
	Errors  []SyncError
}

// RunAll executes all enabled entries. progress may be nil.
func RunAll(cfg *config.Config, progress chan<- ProgressMsg) ([]Result, error) {
	var results []Result
	for _, entry := range cfg.Entries {
		if !entry.Enabled {
			continue
		}
		r, err := RunEntry(entry, progress)
		if err != nil {
			return results, fmt.Errorf("entry %q: %w", entry.Name, err)
		}
		results = append(results, r)
	}
	return results, nil
}

// RunEntry copies source to each target.
func RunEntry(entry config.SyncEntry, progress chan<- ProgressMsg) (Result, error) {
	result := Result{Entry: entry}

	srcInfo, err := os.Stat(entry.Source)
	if err != nil {
		return result, fmt.Errorf("stat source: %w", err)
	}

	for _, target := range entry.Targets {
		var copyErr error
		if srcInfo.IsDir() {
			copyErr = copyDir(entry.Source, target, &result, entry.Name, progress)
		} else {
			dest := filepath.Join(target, filepath.Base(entry.Source))
			copyErr = copyFile(entry.Source, dest)
			if progress != nil {
				progress <- ProgressMsg{EntryName: entry.Name, FilePath: entry.Source, Err: copyErr}
			}
			if copyErr != nil {
				result.Errors = append(result.Errors, SyncError{Path: entry.Source, Err: copyErr})
			} else {
				result.Copied++
			}
		}
		if copyErr != nil && srcInfo.IsDir() {
			// dir errors already recorded per-file; skip
			_ = copyErr
		}
	}

	if progress != nil {
		progress <- ProgressMsg{EntryName: entry.Name, Done: true}
	}
	return result, nil
}

func copyDir(src, dst string, result *Result, entryName string, progress chan<- ProgressMsg) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}
		copyErr := copyFile(path, destPath)
		if progress != nil {
			progress <- ProgressMsg{EntryName: entryName, FilePath: path, Err: copyErr}
		}
		if copyErr != nil {
			result.Errors = append(result.Errors, SyncError{Path: path, Err: copyErr})
		} else {
			result.Copied++
		}
		return nil
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
