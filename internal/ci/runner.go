package ci

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ─── Capabilities detection ───────────────────────────────────────────────────

func detectOS() string {
	return runtime.GOOS
}

func detectCPU() string {
	return runtime.GOARCH
}

func detectDocker() bool {
	cmd := exec.Command("docker", "info")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func detectPython() bool {
	for _, py := range []string{"python3", "python", "py"} {
		out, err := exec.Command(py, "--version").CombinedOutput()
		if err == nil && strings.Contains(string(out), "Python 3") {
			return true
		}
	}
	return false
}

func localIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

func findPython() string {
	for _, py := range []string{"python3", "python", "py"} {
		out, err := exec.Command(py, "--version").CombinedOutput()
		if err == nil && strings.Contains(string(out), "Python 3") {
			return py
		}
	}
	return "python3"
}

// ─── Runner client ────────────────────────────────────────────────────────────

type runnerClient struct {
	serverURL string
	runnerID  string
	name      string
	httpCl    *http.Client
}

func newRunnerClient(host string, port int, name string) *runnerClient {
	return &runnerClient{
		serverURL: fmt.Sprintf("http://%s:%d", host, port),
		name:      name,
		httpCl:    &http.Client{Timeout: 35 * time.Second},
	}
}

func (rc *runnerClient) register() error {
	req := RegisterRequest{
		Name:      rc.name,
		OS:        detectOS(),
		CPU:       detectCPU(),
		HasDocker: detectDocker(),
		HasPython: detectPython(),
		IP:        localIP(),
	}
	body, _ := json.Marshal(req)
	resp, err := rc.httpCl.Post(rc.serverURL+"/api/runner/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}
	defer resp.Body.Close()
	var res map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return fmt.Errorf("register decode: %w", err)
	}
	rc.runnerID = res["runner_id"]
	if rc.runnerID == "" {
		return fmt.Errorf("register: no runner_id in response")
	}
	return nil
}

func (rc *runnerClient) heartbeat() {
	url := fmt.Sprintf("%s/api/runner/%s/heartbeat", rc.serverURL, rc.runnerID)
	resp, err := rc.httpCl.Post(url, "application/json", nil)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (rc *runnerClient) poll() (*BuildCommand, error) {
	url := fmt.Sprintf("%s/api/runner/%s/poll", rc.serverURL, rc.runnerID)
	resp, err := rc.httpCl.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if string(data) == "null\n" || string(data) == "null" {
		return nil, nil
	}
	var cmd BuildCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		return nil, err
	}
	if cmd.BuildID == "" {
		return nil, nil
	}
	return &cmd, nil
}

func (rc *runnerClient) sendLog(buildID, line string) {
	entry := LogEntry{BuildID: buildID, Line: line}
	body, _ := json.Marshal(entry)
	url := fmt.Sprintf("%s/api/runner/%s/log", rc.serverURL, rc.runnerID)
	resp, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return
	}
	r, err := rc.httpCl.Do(resp)
	if err != nil {
		return
	}
	r.Body.Close()
}

func (rc *runnerClient) sendBuildStarted(buildID string) {
	body, _ := json.Marshal(map[string]string{"build_id": buildID})
	apiURL := fmt.Sprintf("%s/api/runner/%s/build-started", rc.serverURL, rc.runnerID)
	req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := rc.httpCl.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (rc *runnerClient) sendDone(buildID string, success bool) {
	done := BuildDone{BuildID: buildID, Success: success}
	body, _ := json.Marshal(done)
	url := fmt.Sprintf("%s/api/runner/%s/build-done", rc.serverURL, rc.runnerID)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := rc.httpCl.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (rc *runnerClient) uploadArtifact(buildID, projectName, filePath string) {
	f, err := os.Open(filePath)
	if err != nil {
		rc.sendLog(buildID, fmt.Sprintf("[runner] cannot open artifact %s: %v", filePath, err))
		return
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("build_id", buildID)         //nolint:errcheck
	mw.WriteField("project_name", projectName) //nolint:errcheck
	fw, err := mw.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		rc.sendLog(buildID, fmt.Sprintf("[runner] multipart error: %v", err))
		return
	}
	if _, err := io.Copy(fw, f); err != nil {
		rc.sendLog(buildID, fmt.Sprintf("[runner] copy error: %v", err))
		return
	}
	mw.Close()

	apiURL := fmt.Sprintf("%s/api/runner/%s/artifact", rc.serverURL, rc.runnerID)
	req, _ := http.NewRequest("POST", apiURL, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	cl := &http.Client{Timeout: 5 * time.Minute}
	resp, err := cl.Do(req)
	if err != nil {
		rc.sendLog(buildID, fmt.Sprintf("[runner] upload failed: %v", err))
		return
	}
	resp.Body.Close()
	rc.sendLog(buildID, fmt.Sprintf("[runner] uploaded artifact: %s", filepath.Base(filePath)))
}

// ─── Build execution ──────────────────────────────────────────────────────────

func injectCredentials(gitURL, user, pass string) string {
	if user == "" && pass == "" {
		return gitURL
	}
	u, err := url.Parse(gitURL)
	if err != nil {
		return gitURL
	}
	u.User = url.UserPassword(user, pass)
	return u.String()
}

// globFiles returns files matching a shell-style glob pattern rooted at dir.
func globFiles(dir, pattern string) []string {
	var matches []string
	// Handle absolute and relative patterns
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(dir, pattern)
	}
	found, err := filepath.Glob(pattern)
	if err == nil {
		matches = append(matches, found...)
	}
	return matches
}

func (rc *runnerClient) executeBuild(cmd BuildCommand) {
	buildID := cmd.BuildID
	rc.sendBuildStarted(buildID)
	rc.sendLog(buildID, fmt.Sprintf("[runner] starting build %s on %s/%s", buildID, detectOS(), detectCPU()))

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "babi-ci-*")
	if err != nil {
		rc.sendLog(buildID, fmt.Sprintf("[runner] failed to create temp dir: %v", err))
		rc.sendDone(buildID, false)
		return
	}
	defer os.RemoveAll(tmpDir)

	// Remove the temp dir so git clone can create it fresh (avoids "already exists" errors)
	if err := os.RemoveAll(tmpDir); err != nil {
		rc.sendLog(buildID, fmt.Sprintf("[runner] failed to remove clone dir: %v", err))
		rc.sendDone(buildID, false)
		return
	}

	// Clone repository
	cloneURL := injectCredentials(cmd.GitURL, cmd.GitUser, cmd.GitPass)
	rc.sendLog(buildID, fmt.Sprintf("[runner] cloning %s", sanitizeURL(cmd.GitURL)))
	cloneCmd := exec.Command("git", "clone", "--depth=1", cloneURL, tmpDir)
	cloneOut, err := cloneCmd.CombinedOutput()
	for _, line := range strings.Split(string(cloneOut), "\n") {
		if strings.TrimSpace(line) != "" {
			rc.sendLog(buildID, "[git] "+line)
		}
	}
	if err != nil {
		rc.sendLog(buildID, fmt.Sprintf("[runner] git clone failed: %v", err))
		rc.sendDone(buildID, false)
		return
	}

	// Inject project_name into params so babi_build.py can use it
	if cmd.Params == nil {
		cmd.Params = make(map[string]interface{})
	}
	if _, exists := cmd.Params["project_name"]; !exists && cmd.ProjectName != "" {
		cmd.Params["project_name"] = cmd.ProjectName
	}

	// Write params.json
	paramsData, _ := json.MarshalIndent(cmd.Params, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "params.json"), paramsData, 0644); err != nil {
		rc.sendLog(buildID, fmt.Sprintf("[runner] failed to write params.json: %v", err))
		rc.sendDone(buildID, false)
		return
	}
	rc.sendLog(buildID, "[runner] wrote params.json")

	// Check babi_build.py exists
	buildScript := filepath.Join(tmpDir, "babi_build.py")
	if _, err := os.Stat(buildScript); err != nil {
		rc.sendLog(buildID, "[runner] babi_build.py not found in repository root")
		rc.sendDone(buildID, false)
		return
	}

	// Run babi_build.py and stream output
	python := findPython()
	rc.sendLog(buildID, fmt.Sprintf("[runner] running: %s babi_build.py params.json", python))
	buildCmd := exec.Command(python, "babi_build.py", "params.json")
	buildCmd.Dir = tmpDir

	stdout, _ := buildCmd.StdoutPipe()
	stderr, _ := buildCmd.StderrPipe()

	if err := buildCmd.Start(); err != nil {
		rc.sendLog(buildID, fmt.Sprintf("[runner] failed to start build: %v", err))
		rc.sendDone(buildID, false)
		return
	}

	// Stream stdout and stderr concurrently
	done := make(chan struct{}, 2)
	streamLines := func(r io.Reader, prefix string) {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 4096)
		var partial string
		for {
			n, err := r.Read(buf)
			if n > 0 {
				partial += string(buf[:n])
				for {
					idx := strings.IndexByte(partial, '\n')
					if idx < 0 {
						break
					}
					line := strings.TrimRight(partial[:idx], "\r")
					partial = partial[idx+1:]
					rc.sendLog(buildID, prefix+line)
				}
			}
			if err != nil {
				if partial != "" {
					rc.sendLog(buildID, prefix+partial)
				}
				break
			}
		}
	}

	go streamLines(stdout, "")
	go streamLines(stderr, "[stderr] ")
	<-done
	<-done

	buildErr := buildCmd.Wait()
	success := buildErr == nil

	if buildErr != nil {
		rc.sendLog(buildID, fmt.Sprintf("[runner] build failed: %v", buildErr))
	} else {
		rc.sendLog(buildID, "[runner] build successful, collecting artifacts...")
	}

	// Collect and upload artifacts
	if cmd.Glob != "" {
		matches := globFiles(tmpDir, cmd.Glob)
		if len(matches) == 0 {
			rc.sendLog(buildID, fmt.Sprintf("[runner] no files matched glob: %s", cmd.Glob))
		}
		for _, m := range matches {
			rc.uploadArtifact(buildID, cmd.ProjectName, m)
		}
	}

	rc.sendDone(buildID, success)
}

// sanitizeURL removes credentials from a URL for logging.
func sanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.User = nil
	return u.String()
}

// ─── Runner main loop ─────────────────────────────────────────────────────────

// RunRunner starts the runner: registers with server and enters the poll loop.
func RunRunner(cfg RunnerConfig) error {
	host := cfg.Server.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Server.Port
	if port == 0 {
		port = 8767
	}
	name := cfg.Name
	if name == "" {
		username := os.Getenv("USER")
		if username == "" {
			username = os.Getenv("USERNAME") // Windows
		}
		name = detectOS() + username
	}

	// Auto-register name: {name}-{cpu}-{has_docker}-{os}
	hasDocker := detectDocker()
	dockerStr := "nodock"
	if hasDocker {
		dockerStr = "docker"
	}
	regName := fmt.Sprintf("%s-%s-%s-%s", name, detectCPU(), dockerStr, detectOS())

	rc := newRunnerClient(host, port, regName)

	// Register with retries
	for {
		if err := rc.register(); err != nil {
			fmt.Printf("[runner] register failed: %v — retrying in 5s\n", err)
			time.Sleep(5 * time.Second)
			continue
		}
		fmt.Printf("[runner] registered as %s (id=%s) at %s:%d\n", regName, rc.runnerID, host, port)
		break
	}

	// Heartbeat goroutine
	go func() {
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		for range tick.C {
			rc.heartbeat()
		}
	}()

	// Poll loop
	for {
		cmd, err := rc.poll()
		if err != nil {
			fmt.Printf("[runner] poll error: %v — retrying in 3s\n", err)
			time.Sleep(3 * time.Second)
			continue
		}
		if cmd != nil {
			fmt.Printf("[runner] received build %s\n", cmd.BuildID)
			go rc.executeBuild(*cmd)
		}
	}
}
