package ci

import (
	"embed"
	"encoding/json"
	"io/fs"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

//go:embed static
var staticFiles embed.FS

// Command returns the top-level "babi ci" command.
func Command() *cobra.Command {
	root := &cobra.Command{
		Use:   "ci",
		Short: "Local CI server, runner, and project scaffolding",
		Long: `babi ci — local-network CI tooling.

  babi ci init                 # scaffold a hello-world C/CMake project
  babi ci server               # start the CI server (port 8767)
  babi ci runner               # start a CI runner (connects to localhost:8767)
  babi ci runner --config f.json`,
	}
	root.AddCommand(
		buildInitCmd(),
		buildServerCmd(),
		buildRunnerCmd(),
	)
	return root
}

// ─── babi ci init ─────────────────────────────────────────────────────────────

func buildInitCmd() *cobra.Command {
	var name string
	var bare bool
	c := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a hello-world C/CMake project with babi_build.py",
		Long: `Create a hello-world C project with CMake and a cross-platform babi_build.py.

  babi ci init                  # creates ./hello/
  babi ci init --name my-app    # creates ./my-app/
  babi ci init --bare           # writes only babi_build.py to current directory`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if bare {
				return writeBuildScriptOnly()
			}
			if name == "" {
				name = "hello"
			}
			return scaffoldCProject(name)
		},
	}
	c.Flags().StringVarP(&name, "name", "n", "hello", "project directory name")
	c.Flags().BoolVar(&bare, "bare", false, "write only babi_build.py to the current directory")
	return c
}

func writeBuildScriptOnly() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	dest := filepath.Join(wd, "babi_build.py")
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("babi_build.py already exists in current directory")
	}
	if err := os.WriteFile(dest, []byte(babiBuildPy), 0644); err != nil {
		return fmt.Errorf("write babi_build.py: %w", err)
	}
	fmt.Printf("[babi] wrote %s\n", dest)
	return nil
}

func scaffoldCProject(name string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	projectDir := filepath.Join(wd, name)
	srcDir := filepath.Join(projectDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	readme := "# " + name + "\n\nHello world C project scaffolded by `babi ci init`.\n\n## Build\n\n```sh\npython3 babi_build.py\n```\n"
	files := [][2]string{
		{filepath.Join(projectDir, "CMakeLists.txt"), cmakelists},
		{filepath.Join(srcDir, "main.c"), mainC},
		{filepath.Join(projectDir, "babi_build.py"), babiBuildPy},
		{filepath.Join(projectDir, ".gitignore"), ciGitignore},
		{filepath.Join(projectDir, "README.md"), readme},
	}
	for _, f := range files {
		if err := os.WriteFile(f[0], []byte(f[1]), 0644); err != nil {
			return fmt.Errorf("write %s: %w", f[0], err)
		}
		fmt.Printf("[babi] wrote %s\n", f[0])
	}

	// git init
	gitCmd := exec.Command("git", "init", "-b", "main", projectDir)
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	if err := gitCmd.Run(); err != nil {
		fmt.Printf("[babi] warning: git init failed: %v\n", err)
	}

	fmt.Printf("\n[babi] project scaffolded at ./%s\n", name)
	fmt.Printf("       Run: cd %s && python3 babi_build.py\n", name)
	return nil
}

// ─── babi ci server ───────────────────────────────────────────────────────────

func buildServerCmd() *cobra.Command {
	var port int
	var projectsFile string
	var artifactsDir string
	var dataDir string
	var noLocalRunner bool

	c := &cobra.Command{
		Use:   "server",
		Short: "Start the CI server",
		Long: `Start the babi CI server.

  babi ci server
  babi ci server --port 8767 --projects ./projects.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(port, projectsFile, artifactsDir, dataDir, !noLocalRunner)
		},
	}
	c.Flags().IntVarP(&port, "port", "p", 8767, "port to listen on")
	c.Flags().StringVar(&projectsFile, "projects", "projects.json", "path to projects.json")
	c.Flags().StringVar(&artifactsDir, "artifacts", "artifacts", "directory to save build artifacts")
	c.Flags().StringVar(&dataDir, "data", ".", "directory to store builds.json and logs/")
	c.Flags().BoolVar(&noLocalRunner, "no-local-runner", false, "do not spawn a local runner")
	return c
}

func runServer(port int, projectsFile, artifactsDir, dataDir string, spawnLocalRunner bool) error {
	// Ensure directories exist
	if err := os.MkdirAll(artifactsDir, 0755); err != nil {
		return fmt.Errorf("create artifacts dir: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	absArtifacts, err := filepath.Abs(artifactsDir)
	if err != nil {
		return err
	}
	absData, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}

	srv := newCIServer(projectsFile, absArtifacts, absData)

	// Serve embedded static files under the "static/" sub-directory.
	// http.FS(sub) serves the files at their paths relative to sub.
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("static FS: %w", err)
	}

	mux := srv.handler(http.FS(sub))

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("[server] babi CI server listening on http://localhost%s\n", addr)
	fmt.Printf("[server] projects:  %s\n", projectsFile)
	fmt.Printf("[server] artifacts: %s\n", absArtifacts)
	fmt.Printf("[server] data dir:  %s\n", absData)

	// Spawn local runner as a separate process
	if spawnLocalRunner {
		go spawnLocalRunnerProcess(port)
	}

	return http.ListenAndServe(addr, mux)
}

func spawnLocalRunnerProcess(serverPort int) {
	// Write a temp config
	type serverCfg struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	type cfg struct {
		Server serverCfg `json:"server"`
		Name   string    `json:"name"`
	}
	localName := "local-" + runtime.GOOS
	c := cfg{
		Server: serverCfg{Host: "localhost", Port: serverPort},
		Name:   localName,
	}
	data, _ := json.Marshal(c)
	cfgPath := filepath.Join(os.TempDir(), fmt.Sprintf("babi-ci-local-%d.json", serverPort))
	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		fmt.Printf("[server] failed to write local runner config: %v\n", err)
		return
	}

	// Wait a moment for server to be ready
	time.Sleep(500 * time.Millisecond)

	self, err := os.Executable()
	if err != nil {
		fmt.Printf("[server] cannot find own executable: %v\n", err)
		return
	}

	for {
		cmd := exec.Command(self, "ci", "runner", "--config", cfgPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("[server] local runner exited (%v), restarting in 5s...\n", err)
			time.Sleep(5 * time.Second)
		}
	}
}

// ─── babi ci runner ───────────────────────────────────────────────────────────

func buildRunnerCmd() *cobra.Command {
	var configFile string

	c := &cobra.Command{
		Use:   "runner",
		Short: "Start a CI runner",
		Long: `Start a CI runner that connects to the CI server.

  babi ci runner                       # connects to localhost:8767
  babi ci runner --config runner.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadRunnerConfig(configFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			return RunRunner(cfg)
		},
	}
	c.Flags().StringVar(&configFile, "config", "", "path to runner config JSON file")
	return c
}

func loadRunnerConfig(path string) (RunnerConfig, error) {
	var cfg RunnerConfig
	// Defaults
	cfg.Server.Host = "localhost"
	cfg.Server.Port = 8767

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	// Apply defaults for missing fields
	if cfg.Server.Host == "" {
		cfg.Server.Host = "localhost"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8767
	}
	return cfg, nil
}
