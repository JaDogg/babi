package tag

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	cc "github.com/jadogg/babi/internal/clicolor"
)

// Command returns the "babi tag" cobra command tree.
func Command() *cobra.Command {
	root := &cobra.Command{
		Use:   "tag [major|minor|patch]",
		Short: "Bump the version tag and push it",
		Long: `Find the latest semver tag, increment it, create an annotated tag, and push.

  babi tag                        # patch bump: v1.2.3 → v1.2.4
  babi tag minor                  # minor bump: v1.2.3 → v1.3.0
  babi tag major                  # major bump: v1.2.3 → v2.0.0
  babi tag set v1.0.0             # hardcode an exact tag
  babi tag -m "my release note"   # custom annotation message
  babi tag --dry-run              # preview without creating or pushing

If no tag exists the first tag will be v0.0.1 regardless of bump type.
Without -m the annotation is auto-generated from commits since the previous tag.`,
		Args: cobra.MaximumNArgs(1),
		RunE: run,
	}
	root.Flags().Bool("dry-run", false, "print the new tag without creating or pushing")
	root.Flags().StringP("message", "m", "", "custom annotation message (skips auto-generated commit log)")

	set := &cobra.Command{
		Use:   "set <tag>",
		Short: "Create and push an exact tag (e.g. v1.0.0)",
		Args:  cobra.ExactArgs(1),
		RunE:  runSet,
	}
	set.Flags().Bool("dry-run", false, "print the tag without creating or pushing")
	set.Flags().StringP("message", "m", "", "custom annotation message")

	root.AddCommand(set)
	return root
}

func run(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	repoDir, err := gitRoot(cwd)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	bump := "patch"
	if len(args) == 1 {
		switch strings.ToLower(args[0]) {
		case "major", "minor", "patch":
			bump = strings.ToLower(args[0])
		default:
			return fmt.Errorf("unknown bump type %q — use major, minor, or patch", args[0])
		}
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	customMsg, _ := cmd.Flags().GetString("message")

	last, err := latestTag(repoDir)
	if err != nil {
		return fmt.Errorf("reading tags: %w", err)
	}

	var newTag string
	if last == "" {
		newTag = "v0.0.1"
	} else {
		newTag, err = bumpVersion(last, bump)
		if err != nil {
			return fmt.Errorf("parsing tag %q: %w", last, err)
		}
	}

	// Build annotation: custom message takes priority over auto-generated log.
	var annotation string
	if customMsg != "" {
		annotation = buildAnnotation(newTag, last, customMsg)
	} else {
		annotation = buildAnnotation(newTag, last, commitLog(repoDir, last))
	}

	// Print summary.
	fmt.Println()
	if last == "" {
		fmt.Printf("  %s  %s\n", cc.Dim("previous:"), cc.Dim("(none)"))
	} else {
		fmt.Printf("  %s  %s\n", cc.Dim("previous:"), cc.Yellow(last))
	}
	fmt.Printf("  %s  %s  %s\n", cc.Dim("new tag: "), cc.BoldGreen(newTag), cc.Dim("("+bump+" bump)"))
	fmt.Println()

	if dryRun {
		fmt.Println(cc.Dim("--- annotation ---"))
		fmt.Println(annotation)
		fmt.Println(cc.Dim("------------------"))
		fmt.Println(cc.BoldYellow("dry-run: no tag created"))
		return nil
	}

	// Create annotated tag.
	if out, err := git(repoDir, "tag", "-a", newTag, "-m", annotation); err != nil {
		return fmt.Errorf("git tag: %s\n%s", err, out)
	}
	fmt.Printf("  %s %s\n", cc.BoldGreen("✓ created"), cc.Bold(newTag))

	// Push the tag.
	fmt.Printf("  %s %s\n", cc.Dim("pushing"), cc.Dim("origin "+newTag))
	if out, err := git(repoDir, "push", "origin", newTag); err != nil {
		return fmt.Errorf("git push: %s\n%s", err, out)
	}
	fmt.Printf("  %s %s\n", cc.BoldGreen("✓ pushed"), cc.Bold(newTag))
	fmt.Println()
	return nil
}

// runSet handles "babi tag set <tag>".
func runSet(cmd *cobra.Command, args []string) error {
	newTag := args[0]

	// Require a leading "v" and valid semver so the tag stays consistent.
	if _, err := parseSemver(newTag); err != nil {
		return fmt.Errorf("invalid semver tag %q — expected format vX.Y.Z: %w", newTag, err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	repoDir, err := gitRoot(cwd)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	customMsg, _ := cmd.Flags().GetString("message")

	last, _ := latestTag(repoDir)

	var annotation string
	if customMsg != "" {
		annotation = buildAnnotation(newTag, last, customMsg)
	} else {
		annotation = buildAnnotation(newTag, last, commitLog(repoDir, last))
	}

	fmt.Println()
	if last != "" {
		fmt.Printf("  %s  %s\n", cc.Dim("previous:"), cc.Yellow(last))
	}
	fmt.Printf("  %s  %s\n", cc.Dim("new tag: "), cc.BoldGreen(newTag))
	fmt.Println()

	if dryRun {
		fmt.Println(cc.Dim("--- annotation ---"))
		fmt.Println(annotation)
		fmt.Println(cc.Dim("------------------"))
		fmt.Println(cc.BoldYellow("dry-run: no tag created"))
		return nil
	}

	if out, err := git(repoDir, "tag", "-a", newTag, "-m", annotation); err != nil {
		return fmt.Errorf("git tag: %s\n%s", err, out)
	}
	fmt.Printf("  %s %s\n", cc.BoldGreen("✓ created"), cc.Bold(newTag))

	fmt.Printf("  %s %s\n", cc.Dim("pushing"), cc.Dim("origin "+newTag))
	if out, err := git(repoDir, "push", "origin", newTag); err != nil {
		return fmt.Errorf("git push: %s\n%s", err, out)
	}
	fmt.Printf("  %s %s\n", cc.BoldGreen("✓ pushed"), cc.Bold(newTag))
	fmt.Println()
	return nil
}

// ─── git helpers ──────────────────────────────────────────────────────────────

func gitRoot(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// latestTag returns the most recent semver tag (vX.Y.Z), or "" if none exist.
func latestTag(repoDir string) (string, error) {
	// --sort=-version:refname sorts newest semver tag first.
	out, err := exec.Command(
		"git", "-C", repoDir,
		"tag", "-l", "--sort=-version:refname", "v[0-9]*.[0-9]*.[0-9]*",
	).Output()
	if err != nil {
		// git tag never fails just because there are no tags; treat any error as fatal.
		return "", err
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", nil
	}
	return lines[0], nil
}

// commitLog returns a formatted list of commits since fromTag (or recent commits
// if fromTag is empty), ready to embed in a tag annotation.
func commitLog(repoDir, fromTag string) string {
	var args []string
	if fromTag != "" {
		args = []string{"-C", repoDir, "log", fromTag + "..HEAD", "--pretty=format:  %h %s"}
	} else {
		args = []string{"-C", repoDir, "log", "-20", "--pretty=format:  %h %s"}
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return "  (no commits)"
	}
	return strings.TrimRight(string(out), "\n")
}

// git runs a git command in repoDir and returns combined output.
func git(repoDir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoDir}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ─── version logic ────────────────────────────────────────────────────────────

type semver struct {
	major, minor, patch int
	prefix              string // usually "v"
}

func parseSemver(tag string) (semver, error) {
	prefix := ""
	s := tag
	if strings.HasPrefix(s, "v") {
		prefix = "v"
		s = s[1:]
	}
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("expected vX.Y.Z, got %q", tag)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, fmt.Errorf("bad major %q", parts[0])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semver{}, fmt.Errorf("bad minor %q", parts[1])
	}
	// Strip any pre-release suffix from patch (e.g. "3-rc1" → 3)
	patchStr := strings.SplitN(parts[2], "-", 2)[0]
	patch, err := strconv.Atoi(patchStr)
	if err != nil {
		return semver{}, fmt.Errorf("bad patch %q", parts[2])
	}
	return semver{major: major, minor: minor, patch: patch, prefix: prefix}, nil
}

func (v semver) String() string {
	return fmt.Sprintf("%s%d.%d.%d", v.prefix, v.major, v.minor, v.patch)
}

func bumpVersion(tag, bump string) (string, error) {
	v, err := parseSemver(tag)
	if err != nil {
		return "", err
	}
	switch bump {
	case "major":
		v.major++
		v.minor = 0
		v.patch = 0
	case "minor":
		v.minor++
		v.patch = 0
	default: // patch
		v.patch++
	}
	return v.String(), nil
}

func buildAnnotation(newTag, lastTag, log string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Release %s\n\n", newTag)
	if lastTag == "" {
		sb.WriteString("Changes:\n")
	} else {
		fmt.Fprintf(&sb, "Changes since %s:\n", lastTag)
	}
	sb.WriteString(log)
	return sb.String()
}
