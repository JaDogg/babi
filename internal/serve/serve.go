package serve

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed dir.html
var dirHTML string

// Command returns the "babi serve" cobra command tree.
func Command() *cobra.Command {
	var port int
	var expose bool

	root := &cobra.Command{
		Use:   "serve",
		Short: "HTTP server utilities",
		Long:  "Serve files over HTTP.\n\n  babi serve web [dir]   # static file server\n  babi serve dir [dir]   # interactive directory browser",
	}
	root.PersistentFlags().IntVarP(&port, "port", "p", 8080, "port to listen on")
	root.PersistentFlags().BoolVar(&expose, "expose", false, "expose to network (bind 0.0.0.0 instead of 127.0.0.1)")

	webCmd := &cobra.Command{
		Use:   "web [dir]",
		Short: "Serve static files from a directory (serves index.html etc.)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			host, addr := resolveAddr(expose, port)
			_ = host
			fmt.Printf("Serving  %s\n", abs)
			fmt.Printf("Address  http://%s\n", addr)
			return http.ListenAndServe(addr, http.FileServer(http.Dir(abs)))
		},
	}

	dirCmd := &cobra.Command{
		Use:   "dir [dir]",
		Short: "Interactive directory browser with video streaming and download",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			_, addr := resolveAddr(expose, port)
			return runDir(addr, abs)
		},
	}

	root.AddCommand(webCmd, dirCmd)
	return root
}

func resolveAddr(expose bool, port int) (host, addr string) {
	host = "127.0.0.1"
	if expose {
		host = "0.0.0.0"
	}
	addr = fmt.Sprintf("%s:%d", host, port)
	return
}

// ─── dir server ──────────────────────────────────────────────────────────────

type entry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
	Mime    string `json:"mime,omitempty"`
}

func runDir(addr, root string) error {
	mux := http.NewServeMux()

	// SPA shell
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, dirHTML)
	})

	// Directory listing API
	mux.HandleFunc("/api/ls", func(w http.ResponseWriter, r *http.Request) {
		rel := filepath.FromSlash(r.URL.Query().Get("path"))
		abs, err := safeJoin(root, rel)
		if err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		infos, err := os.ReadDir(abs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		entries := make([]entry, 0, len(infos))
		for _, info := range infos {
			if strings.HasPrefix(info.Name(), ".") {
				continue
			}
			e := entry{Name: info.Name(), IsDir: info.IsDir()}
			if fi, err2 := info.Info(); err2 == nil {
				e.Size = fi.Size()
				e.ModTime = fi.ModTime().Unix()
			}
			if !info.IsDir() {
				e.Mime = mime.TypeByExtension(filepath.Ext(info.Name()))
			}
			entries = append(entries, e)
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir != entries[j].IsDir {
				return entries[i].IsDir
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	})

	// File server — supports Range requests for video streaming
	mux.HandleFunc("/f/", func(w http.ResponseWriter, r *http.Request) {
		rel := filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/f/"))
		// Reject hidden path components
		for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
			if strings.HasPrefix(part, ".") {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		abs, err := safeJoin(root, rel)
		if err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		f, err := os.Open(abs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil || fi.IsDir() {
			http.Error(w, "not a file", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("dl") == "1" {
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, fi.Name()))
		}
		// ServeContent handles Range, ETag, Last-Modified automatically
		http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
	})

	fmt.Printf("Directory browser  http://%s\n", addr)
	fmt.Printf("Serving            %s\n", root)
	return http.ListenAndServe(addr, mux)
}

// safeJoin joins root and rel, returning an error if the result escapes root.
func safeJoin(root, rel string) (string, error) {
	abs := filepath.Clean(filepath.Join(root, rel))
	rel2, err := filepath.Rel(root, abs)
	if err != nil {
		return "", fmt.Errorf("forbidden")
	}
	if strings.HasPrefix(rel2, "..") {
		return "", fmt.Errorf("forbidden")
	}
	return abs, nil
}
