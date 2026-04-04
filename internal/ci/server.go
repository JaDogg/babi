package ci

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─── ID generation ────────────────────────────────────────────────────────────

func newID() string {
	b := make([]byte, 8)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}

// ─── Log broker (fan-out to SSE subscribers) ─────────────────────────────────

type logBroker struct {
	mu      sync.Mutex
	subs    map[string][]chan string // build_id → subscriber channels
	history map[string][]string     // build_id → all lines (for late joiners)
	closed  map[string]bool         // build_id → build finished
}

func newLogBroker() *logBroker {
	return &logBroker{
		subs:    make(map[string][]chan string),
		history: make(map[string][]string),
		closed:  make(map[string]bool),
	}
}

func (lb *logBroker) publish(buildID, line string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.history[buildID] = append(lb.history[buildID], line)
	for _, ch := range lb.subs[buildID] {
		select {
		case ch <- line:
		default: // drop if slow consumer
		}
	}
}

func (lb *logBroker) close(buildID string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.closed[buildID] = true
	for _, ch := range lb.subs[buildID] {
		close(ch)
	}
	lb.subs[buildID] = nil
}

func (lb *logBroker) subscribe(buildID string) (chan string, []string, bool) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	hist := make([]string, len(lb.history[buildID]))
	copy(hist, lb.history[buildID])
	if lb.closed[buildID] {
		return nil, hist, true
	}
	ch := make(chan string, 256)
	lb.subs[buildID] = append(lb.subs[buildID], ch)
	return ch, hist, false
}

func (lb *logBroker) unsubscribe(buildID string, ch chan string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	subs := lb.subs[buildID]
	for i, s := range subs {
		if s == ch {
			lb.subs[buildID] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

// ─── Server ───────────────────────────────────────────────────────────────────

type connectedRunner struct {
	Runner
	jobCh chan BuildCommand
}

// persistedBuild is the on-disk representation of a build (includes artifact paths).
type persistedBuild struct {
	Build
	Artifacts []string `json:"artifacts"`
}

type buildsFile struct {
	Builds []persistedBuild `json:"builds"`
}

type ciServer struct {
	mu           sync.RWMutex
	runners      map[string]*connectedRunner // runner_id → state
	builds       map[string]*Build           // build_id → build
	buildsByProj map[string][]*Build         // project_id → builds
	artifacts    map[string][]string         // build_id → file paths
	projects     []Project
	projectsPath string
	artifactsDir string
	dataDir      string // stores builds.json and logs/
	broker       *logBroker
}

func newCIServer(projectsPath, artifactsDir, dataDir string) *ciServer {
	s := &ciServer{
		runners:      make(map[string]*connectedRunner),
		builds:       make(map[string]*Build),
		buildsByProj: make(map[string][]*Build),
		artifacts:    make(map[string][]string),
		projectsPath: projectsPath,
		artifactsDir: artifactsDir,
		dataDir:      dataDir,
		broker:       newLogBroker(),
	}
	s.loadProjects()
	s.loadBuildsFromDisk()
	return s
}

func (s *ciServer) loadProjects() {
	data, err := os.ReadFile(s.projectsPath)
	if err != nil {
		return
	}
	var pf ProjectsFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return
	}
	s.projects = pf.Projects
	// Ensure maps are non-nil
	for i := range s.projects {
		if s.projects[i].Params == nil {
			s.projects[i].Params = make(map[string]interface{})
		}
		if s.projects[i].RunnerParams == nil {
			s.projects[i].RunnerParams = make(map[string]map[string]interface{})
		}
	}
}

func (s *ciServer) saveProjects() {
	pf := ProjectsFile{Projects: s.projects}
	data, _ := json.MarshalIndent(pf, "", "  ")
	os.WriteFile(s.projectsPath, data, 0644) //nolint:errcheck
}

// ─── Build persistence ────────────────────────────────────────────────────────

func (s *ciServer) logsDir() string    { return filepath.Join(s.dataDir, "logs") }
func (s *ciServer) buildsPath() string { return filepath.Join(s.dataDir, "builds.json") }

// logLine publishes to in-memory broker AND appends to the on-disk log file.
func (s *ciServer) logLine(buildID, line string) {
	s.broker.publish(buildID, line)
	logPath := filepath.Join(s.logsDir(), buildID+".log")
	os.MkdirAll(s.logsDir(), 0755) //nolint:errcheck
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	fmt.Fprintln(f, line)
	f.Close()
}

func (s *ciServer) saveBuildsDisk() {
	s.mu.RLock()
	records := make([]persistedBuild, 0, len(s.builds))
	for _, b := range s.builds {
		records = append(records, persistedBuild{
			Build:     *b,
			Artifacts: s.artifacts[b.ID],
		})
	}
	s.mu.RUnlock()

	os.MkdirAll(s.dataDir, 0755) //nolint:errcheck
	data, _ := json.MarshalIndent(buildsFile{Builds: records}, "", "  ")
	os.WriteFile(s.buildsPath(), data, 0644) //nolint:errcheck
}

func (s *ciServer) loadBuildsFromDisk() {
	data, err := os.ReadFile(s.buildsPath())
	if err != nil {
		return
	}
	var bf buildsFile
	if err := json.Unmarshal(data, &bf); err != nil {
		return
	}
	for _, rec := range bf.Builds {
		b := rec.Build // copy

		// Builds that were running/pending when server stopped are now failed
		if b.Status == "running" || b.Status == "pending" {
			b.Status = "failed"
			t := time.Now()
			b.EndedAt = &t
		}

		bp := &b
		s.builds[b.ID] = bp
		s.buildsByProj[b.ProjectID] = append(s.buildsByProj[b.ProjectID], bp)
		if len(rec.Artifacts) > 0 {
			s.artifacts[b.ID] = rec.Artifacts
		}

		// Restore log history into broker from disk
		logPath := filepath.Join(s.logsDir(), b.ID+".log")
		logData, err := os.ReadFile(logPath)
		if err == nil {
			lines := strings.Split(strings.TrimRight(string(logData), "\n"), "\n")
			s.broker.mu.Lock()
			s.broker.history[b.ID] = lines
			s.broker.closed[b.ID] = true // build is done; SSE will replay + fire done
			s.broker.mu.Unlock()
		}
	}
	fmt.Printf("[server] restored %d build(s) from disk\n", len(bf.Builds))
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func readJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Runner API handlers ──────────────────────────────────────────────────────

func (s *ciServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := newID()
	s.mu.Lock()
	s.runners[id] = &connectedRunner{
		Runner: Runner{
			ID:        id,
			Name:      req.Name,
			OS:        req.OS,
			CPU:       req.CPU,
			HasDocker: req.HasDocker,
			HasPython: req.HasPython,
			IP:        req.IP,
			LastSeen:  time.Now(),
			Online:    true,
		},
		jobCh: make(chan BuildCommand, 1),
	}
	s.mu.Unlock()
	fmt.Printf("[server] runner registered: %s (id=%s, os=%s, cpu=%s, docker=%v)\n",
		req.Name, id, req.OS, req.CPU, req.HasDocker)
	writeJSON(w, http.StatusOK, map[string]string{"runner_id": id})
}

func (s *ciServer) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	if rs, ok := s.runners[id]; ok {
		rs.LastSeen = time.Now()
		rs.Online = true
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// handlePoll blocks up to 25s waiting for a build job.
func (s *ciServer) handlePoll(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	rs, ok := s.runners[id]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "runner not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	select {
	case cmd := <-rs.jobCh:
		writeJSON(w, http.StatusOK, cmd)
	case <-ctx.Done():
		writeJSON(w, http.StatusOK, nil)
	}
}

func (s *ciServer) handleLog(w http.ResponseWriter, r *http.Request) {
	var entry LogEntry
	if err := readJSON(r, &entry); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.broker.publish(entry.BuildID, entry.Line)
	w.WriteHeader(http.StatusOK)
}

func (s *ciServer) handleBuildStarted(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BuildID string `json:"build_id"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if b, ok := s.builds[req.BuildID]; ok {
		b.Status = "running"
	}
	s.mu.Unlock()
	go s.saveBuildsDisk()
	w.WriteHeader(http.StatusOK)
}

func (s *ciServer) handleBuildDone(w http.ResponseWriter, r *http.Request) {
	var done BuildDone
	if err := readJSON(r, &done); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if b, ok := s.builds[done.BuildID]; ok {
		t := time.Now()
		b.EndedAt = &t
		if done.Success {
			b.Status = "success"
		} else {
			b.Status = "failed"
		}
	}
	s.mu.Unlock()
	status := "failed"
	if done.Success {
		status = "success"
	}
	s.logLine(done.BuildID, fmt.Sprintf("[babi-ci] build %s", status))
	s.broker.close(done.BuildID)
	go s.saveBuildsDisk()
	w.WriteHeader(http.StatusOK)
}

func (s *ciServer) handleArtifactUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	buildID := r.FormValue("build_id")
	projectName := r.FormValue("project_name")

	fh, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fh.Close()

	date := time.Now().Format("2006-01-02")
	destDir := filepath.Join(s.artifactsDir, sanitizeName(projectName), date)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	destPath := filepath.Join(destDir, filepath.Base(header.Filename))
	f, err := os.Create(destPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	if _, err := io.Copy(f, fh); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	s.artifacts[buildID] = append(s.artifacts[buildID], destPath)
	s.mu.Unlock()

	s.logLine(buildID, fmt.Sprintf("[babi-ci] artifact saved: %s", header.Filename))
	go s.saveBuildsDisk()
	writeJSON(w, http.StatusOK, map[string]string{"path": destPath})
}

// ─── Runner list ──────────────────────────────────────────────────────────────

func (s *ciServer) handleListRunners(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	runners := make([]Runner, 0, len(s.runners))
	for _, rs := range s.runners {
		rs.Online = time.Since(rs.LastSeen) < 30*time.Second
		runners = append(runners, rs.Runner)
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, runners)
}

// ─── Project CRUD ─────────────────────────────────────────────────────────────

func (s *ciServer) handleListProjects(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	projects := make([]Project, len(s.projects))
	copy(projects, s.projects)
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, projects)
}

func (s *ciServer) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var p Project
	if err := readJSON(r, &p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.ID = newID()
	if p.Params == nil {
		p.Params = make(map[string]interface{})
	}
	if p.RunnerParams == nil {
		p.RunnerParams = make(map[string]map[string]interface{})
	}
	s.mu.Lock()
	s.projects = append(s.projects, p)
	s.saveProjects()
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, p)
}

func (s *ciServer) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var p Project
	if err := readJSON(r, &p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.ID = id
	s.mu.Lock()
	found := false
	for i, proj := range s.projects {
		if proj.ID == id {
			s.projects[i] = p
			found = true
			break
		}
	}
	if found {
		s.saveProjects()
	}
	s.mu.Unlock()
	if !found {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *ciServer) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	for i, p := range s.projects {
		if p.ID == id {
			s.projects = append(s.projects[:i], s.projects[i+1:]...)
			s.saveProjects()
			break
		}
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *ciServer) handleRunProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	var proj *Project
	for i := range s.projects {
		if s.projects[i].ID == id {
			cp := s.projects[i]
			proj = &cp
			break
		}
	}
	s.mu.RUnlock()
	if proj == nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	s.mu.RLock()
	var targets []*connectedRunner
	for _, rs := range s.runners {
		if time.Since(rs.LastSeen) >= 30*time.Second {
			continue
		}
		if len(proj.Runners) == 0 {
			targets = append(targets, rs)
		} else {
			for _, name := range proj.Runners {
				if rs.Name == name {
					targets = append(targets, rs)
					break
				}
			}
		}
	}

	// Find configured runners that are offline
	var missingRunners []string
	if len(proj.Runners) > 0 {
		for _, name := range proj.Runners {
			found := false
			for _, t := range targets {
				if t.Name == name {
					found = true
					break
				}
			}
			if !found {
				missingRunners = append(missingRunners, name)
			}
		}
	}
	s.mu.RUnlock()

	type buildResult struct {
		BuildID    string `json:"build_id"`
		RunnerName string `json:"runner_name"`
		Status     string `json:"status"`
	}
	var results []buildResult

	for _, rs := range targets {
		buildID := newID()

		// Merge params: common params + runner-specific (runner takes priority)
		params := make(map[string]interface{})
		for k, v := range proj.Params {
			params[k] = v
		}
		if rp, ok := proj.RunnerParams[rs.Name]; ok {
			for k, v := range rp {
				params[k] = v
			}
		}

		cmd := BuildCommand{
			BuildID:     buildID,
			ProjectName: proj.Name,
			GitURL:      proj.GitURL,
			GitUser:     proj.GitUser,
			GitPass:     proj.GitPass,
			Glob:        proj.FileGlob,
			Params:      params,
		}

		b := &Build{
			ID:          buildID,
			ProjectID:   proj.ID,
			ProjectName: proj.Name,
			RunnerID:    rs.ID,
			RunnerName:  rs.Name,
			Status:      "pending",
			StartedAt:   time.Now(),
		}

		s.mu.Lock()
		s.builds[buildID] = b
		s.buildsByProj[proj.ID] = append(s.buildsByProj[proj.ID], b)
		s.mu.Unlock()

		select {
		case rs.jobCh <- cmd:
			results = append(results, buildResult{buildID, rs.Name, "dispatched"})
		default:
			s.mu.Lock()
			b.Status = "failed"
			t := time.Now()
			b.EndedAt = &t
			s.mu.Unlock()
			s.logLine(buildID, "[babi-ci] runner busy — could not dispatch")
			s.broker.close(buildID)
			results = append(results, buildResult{buildID, rs.Name, "busy"})
		}
	}
	go s.saveBuildsDisk()

	warning := ""
	if len(targets) == 0 {
		warning = "no online runners found"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"builds":          results,
		"warning":         warning,
		"missing_runners": missingRunners,
	})
}

// ─── Build API ────────────────────────────────────────────────────────────────

func (s *ciServer) handleListBuilds(w http.ResponseWriter, r *http.Request) {
	projID := r.URL.Query().Get("project_id")
	s.mu.RLock()
	var builds []*Build
	if projID != "" {
		builds = s.buildsByProj[projID]
	} else {
		for _, b := range s.builds {
			builds = append(builds, b)
		}
	}
	s.mu.RUnlock()
	if builds == nil {
		builds = []*Build{}
	}
	writeJSON(w, http.StatusOK, builds)
}

func (s *ciServer) handleGetBuild(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	b, ok := s.builds[id]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "build not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *ciServer) handleLogStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ch, history, done := s.broker.subscribe(id)

	// Send history first
	for _, line := range history {
		fmt.Fprintf(w, "data: %s\n\n", jsonEscape(line))
		flusher.Flush()
	}

	if done {
		fmt.Fprintf(w, "event: done\ndata: done\n\n")
		flusher.Flush()
		return
	}

	defer s.broker.unsubscribe(id, ch)

	for {
		select {
		case line, open := <-ch:
			if !open {
				fmt.Fprintf(w, "event: done\ndata: done\n\n")
				flusher.Flush()
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", jsonEscape(line))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *ciServer) handleGetArtifacts(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	paths := make([]string, len(s.artifacts[id]))
	copy(paths, s.artifacts[id])
	s.mu.RUnlock()

	type artifactInfo struct {
		Filename string `json:"filename"`
		URL      string `json:"url"`
	}
	var result []artifactInfo
	for _, p := range paths {
		rel := strings.TrimPrefix(p, s.artifactsDir)
		result = append(result, artifactInfo{
			Filename: filepath.Base(p),
			URL:      "/api/artifacts/download?f=" + rel,
		})
	}
	if result == nil {
		result = []artifactInfo{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *ciServer) handleArtifactDownload(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("f")
	// Prevent path traversal
	rel = filepath.Clean("/" + rel)
	absPath := filepath.Join(s.artifactsDir, rel)
	absArtDir, _ := filepath.Abs(s.artifactsDir)
	absFile, _ := filepath.Abs(absPath)
	if !strings.HasPrefix(absFile, absArtDir) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(absFile)))
	http.ServeFile(w, r, absFile)
}

// ─── Misc helpers ─────────────────────────────────────────────────────────────

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ─── HTTP mux setup ───────────────────────────────────────────────────────────

func (s *ciServer) handler(staticFS http.FileSystem) http.Handler {
	mux := http.NewServeMux()

	// Runner API
	mux.HandleFunc("POST /api/runner/register", s.handleRegister)
	mux.HandleFunc("POST /api/runner/{id}/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("GET /api/runner/{id}/poll", s.handlePoll)
	mux.HandleFunc("POST /api/runner/{id}/log", s.handleLog)
	mux.HandleFunc("POST /api/runner/{id}/build-started", s.handleBuildStarted)
	mux.HandleFunc("POST /api/runner/{id}/build-done", s.handleBuildDone)
	mux.HandleFunc("POST /api/runner/{id}/artifact", s.handleArtifactUpload)

	// Runners list
	mux.HandleFunc("GET /api/runners", s.handleListRunners)

	// Projects CRUD
	mux.HandleFunc("GET /api/projects", s.handleListProjects)
	mux.HandleFunc("POST /api/projects", s.handleCreateProject)
	mux.HandleFunc("PUT /api/projects/{id}", s.handleUpdateProject)
	mux.HandleFunc("DELETE /api/projects/{id}", s.handleDeleteProject)
	mux.HandleFunc("POST /api/projects/{id}/run", s.handleRunProject)

	// Builds
	mux.HandleFunc("GET /api/builds", s.handleListBuilds)
	mux.HandleFunc("GET /api/builds/{id}", s.handleGetBuild)
	mux.HandleFunc("GET /api/builds/{id}/logs/stream", s.handleLogStream)
	mux.HandleFunc("GET /api/builds/{id}/artifacts", s.handleGetArtifacts)
	mux.HandleFunc("GET /api/artifacts/download", s.handleArtifactDownload)

	// Static frontend
	mux.Handle("/", http.FileServer(staticFS))

	return corsMiddleware(mux)
}
