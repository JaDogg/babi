// package newproject implements 'babi new'

package newproject

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/spf13/cobra"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func getStr(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

func gitInit(dir string) error {
	cmd := exec.Command("git", "init", "-b", "main", dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	return nil
}

func writeTemplate(path, tmplStr string, data any) error {
	t, err := template.New("").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("internal template error: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := t.Execute(f, data); err != nil {
		return fmt.Errorf("render template: %w", err)
	}
	fmt.Printf("[babi] wrote %s\n", path)
	return nil
}

func Command() *cobra.Command {
	root := buildRoot()
	subs := []*cobra.Command{
		buildPythonUv(),
	}
	root.AddCommand(subs...)
	return root
}
func buildRoot() *cobra.Command {
	return &cobra.Command{
		Use:   "new",
		Short: "Generate a new project",
		Long: `Generate new project files.

  babi new python      # Python + UV`,
	}
}

const pyProjectTmpl = `[project]
name = "{{.Name}}"
version = "{{.Version}}"
description = "Project {{.Name}}"
requires-python = ">=3.14"
dependencies = []

[dependency-groups]
dev = [
    "black>=26.3.0",
    "coverage>=7.13.4",
    "pytest>=9.0.2",
]

[tool.pytest.ini_options]
testpaths = [
    "core/tests",
]
`

const pyProjectMainTmpl = `def main():
    print("Hello {{.Name}}")

if __name__ == "__main__":
    main()

`

const pyTestTmpl = `def test_one():
    assert 1 == 1
`

const readmeTmpl = `# {{.Name}}
`

func buildPythonUv() *cobra.Command {
	c := &cobra.Command{
		Use:   "python",
		Short: "Generate a python project",
		Long: `Genereate a python project that uses uv

  babi new python --name MyApp --version 1.0.0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := getStr(cmd, "name")
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			ver := getStr(cmd, "version")
			if ver == "" {
				ver = "1.0.0"
			}
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			projectDir := filepath.Join(wd, name)
			err = os.MkdirAll(filepath.Join(projectDir, "core", "tests"), 0755)
			if err != nil {
				return err
			}
			templateData := struct {
				Name, Version string
			}{name, ver}
			// README.md
			readmeFile := filepath.Join(projectDir, "README.md")
			err = writeTemplate(readmeFile, readmeTmpl, templateData)
			if err != nil {
				return err
			}
			// pyproject.toml
			projFile := filepath.Join(projectDir, "pyproject.toml")
			err = writeTemplate(projFile, pyProjectTmpl, templateData)
			if err != nil {
				return err
			}
			// main.py
			mainPyFile := filepath.Join(projectDir, "main.py")
			err = writeTemplate(mainPyFile, pyProjectMainTmpl, templateData)
			if err != nil {
				return err
			}
			// sample_test.py
			sampleTestFile := filepath.Join(projectDir, "core", "tests", "sample_test.py")
			err = writeTemplate(sampleTestFile, pyTestTmpl, templateData)
			if err != nil {
				return err
			}
			// git init
			return gitInit(projectDir)

		},
	}
	c.Flags().StringP("name", "n", "", "project_name")
	c.Flags().StringP("version", "v", "1.0.0", "1.0.0")
	return c
}
