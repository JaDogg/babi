// Package ci implements 'babi ci' — a local CI server, runner, and project initializer.
package ci

import "time"

// Runner represents a connected build runner.
type Runner struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OS        string    `json:"os"`
	CPU       string    `json:"cpu"`
	HasDocker bool      `json:"has_docker"`
	HasPython bool      `json:"has_python"`
	IP        string    `json:"ip"`
	LastSeen  time.Time `json:"last_seen"`
	Online    bool      `json:"online"`
}

// Project holds CI project configuration persisted to projects.json.
type Project struct {
	ID           string                            `json:"id"`
	Name         string                            `json:"name"`
	GitURL       string                            `json:"git_url"`
	GitUser      string                            `json:"git_user"`
	GitPass      string                            `json:"git_pass"`
	FileGlob     string                            `json:"file_glob"`
	Params       map[string]interface{}            `json:"params"`
	RunnerParams map[string]map[string]interface{} `json:"runner_params"`
	Runners      []string                          `json:"runners"`
}

// Build represents a single build execution.
type Build struct {
	ID         string     `json:"id"`
	ProjectID  string     `json:"project_id"`
	ProjectName string    `json:"project_name"`
	RunnerID   string     `json:"runner_id"`
	RunnerName string     `json:"runner_name"`
	Status     string     `json:"status"` // pending, running, success, failed
	StartedAt  time.Time  `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
}

// BuildCommand is sent from server to runner to start a build.
type BuildCommand struct {
	BuildID     string                 `json:"build_id"`
	ProjectName string                 `json:"project_name"`
	GitURL      string                 `json:"git_url"`
	GitUser     string                 `json:"git_user"`
	GitPass     string                 `json:"git_pass"`
	Glob        string                 `json:"glob"`
	Params      map[string]interface{} `json:"params"`
}

// RegisterRequest is sent by runner during registration.
type RegisterRequest struct {
	Name      string `json:"name"`
	OS        string `json:"os"`
	CPU       string `json:"cpu"`
	HasDocker bool   `json:"has_docker"`
	HasPython bool   `json:"has_python"`
	IP        string `json:"ip"`
}

// LogEntry is a log line posted from runner to server.
type LogEntry struct {
	BuildID string `json:"build_id"`
	Line    string `json:"line"`
}

// BuildDone signals build completion from runner to server.
type BuildDone struct {
	BuildID string `json:"build_id"`
	Success bool   `json:"success"`
}

// RunnerConfig is the runner's JSON config file format.
type RunnerConfig struct {
	Server struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"server"`
	Name string `json:"name"`
}

// ProjectsFile is the on-disk format for projects.json.
type ProjectsFile struct {
	Projects []Project `json:"projects"`
}
