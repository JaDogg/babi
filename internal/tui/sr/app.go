// Package sr implements a smart rename/delete TUI using bubbletea and lipgloss.
package sr

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

var (
	colorPrimary = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#02A699", Dark: "#04D18A"}
	colorError   = lipgloss.AdaptiveColor{Light: "#CC3333", Dark: "#FF5555"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#767676", Dark: "#626262"}
	colorFaint   = lipgloss.AdaptiveColor{Light: "#DEDEDE", Dark: "#333333"}
	colorFileHit = lipgloss.AdaptiveColor{Light: "#0066CC", Dark: "#6699FF"}
	colorDirHit  = lipgloss.AdaptiveColor{Light: "#007700", Dark: "#44CC44"}

	styleHeaderBg    = lipgloss.NewStyle().Background(colorPrimary)
	styleHeaderTitle = lipgloss.NewStyle().Background(colorPrimary).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 2)
	styleHeaderRight = lipgloss.NewStyle().Background(colorPrimary).Foreground(lipgloss.Color("#CCC8FF")).Padding(0, 2)
	styleFooterBar   = lipgloss.NewStyle().Background(colorFaint).Foreground(colorMuted).Padding(0, 2)
	styleFooterKey   = lipgloss.NewStyle().Background(colorMuted).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1)
	styleFileMatch   = lipgloss.NewStyle().Foreground(colorFileHit).Bold(true)
	styleDirMatch    = lipgloss.NewStyle().Foreground(colorDirHit).Bold(true)
	styleDimRow      = lipgloss.NewStyle().Foreground(colorMuted)
	styleSuccess     = lipgloss.NewStyle().Foreground(colorSuccess)
	styleError       = lipgloss.NewStyle().Foreground(colorError)
	styleSubtle      = lipgloss.NewStyle().Foreground(colorMuted)
	styleArrow       = lipgloss.NewStyle().Foreground(colorMuted)
	styleDelMark     = lipgloss.NewStyle().Foreground(colorError)
	styleToName      = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	styleInputLabel  = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	styleUnchanged   = lipgloss.NewStyle().Foreground(colorMuted)
)

// ---------------------------------------------------------------------------
// Layout helpers
// ---------------------------------------------------------------------------

func renderHeader(width int, title, right string) string {
	titleStr := styleHeaderTitle.Render(title)
	rightStr := styleHeaderRight.Render(right)
	titleW := lipgloss.Width(titleStr)
	rightW := lipgloss.Width(rightStr)
	gap := width - titleW - rightW
	if gap < 0 {
		gap = 0
	}
	fill := styleHeaderBg.Render(strings.Repeat(" ", gap))
	return titleStr + fill + rightStr
}

func renderFooter(width int, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		k := styleFooterKey.Render(pairs[i])
		d := styleSubtle.Render(" " + pairs[i+1] + "  ")
		parts = append(parts, k+d)
	}
	content := strings.Join(parts, "")
	content += strings.Repeat(" ", max(0, width-lipgloss.Width(content)))
	return styleFooterBar.Render(content)
}

func padToHeight(lines []string, height int) []string {
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 3 {
		return string(runes[:n])
	}
	return string(runes[:n-3]) + "..."
}

func padRight(s string, n int) string {
	l := utf8.RuneCountInString(s)
	if l >= n {
		return s
	}
	return s + strings.Repeat(" ", n-l)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// Counter substitution
// ---------------------------------------------------------------------------

// counterRe matches {#} or {#:N} placeholders in replacement strings.
var counterRe = regexp.MustCompile(`\{#(?::(\d+))?\}`)

// substituteCounter replaces {#} and {#:N} in s with the counter value n.
// {#} → plain integer, {#:3} → zero-padded to 3 digits.
func substituteCounter(s string, n int) string {
	return counterRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := counterRe.FindStringSubmatch(m)
		if sub[1] == "" {
			return strconv.Itoa(n)
		}
		digits, _ := strconv.Atoi(sub[1])
		return fmt.Sprintf("%0*d", digits, n)
	})
}

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

type screen int

const (
	screenPattern screen = iota
	screenReplace
	screenConfirm
	screenDone
)

type opType int

const (
	opRename opType = iota
	opDelete
)

// changeEntry describes one pending rename or delete operation.
type changeEntry struct {
	from  string
	to    string // empty when op is delete
	isDir bool
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// Model is the bubbletea model for the smart-rename TUI.
type Model struct {
	dir     string
	entries []os.DirEntry

	screen screen
	op     opType

	findInput    string
	replaceInput string

	findRe  *regexp.Regexp
	findErr string

	changes []changeEntry // confirmed pending operations
	scroll  int           // scroll offset for any list

	results []string // outcome messages shown on the done screen

	width  int
	height int
	ready  bool // true after first WindowSizeMsg; guards against startup garbage
}

// New creates a Model for the given directory.
func New(dir string) (Model, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Model{}, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return Model{}, fmt.Errorf("sr: read dir: %w", err)
	}
	return Model{
		dir:     abs,
		entries: entries,
		screen:  screenPattern,
		op:      opRename,
	}, nil
}

// recompileFindRe recompiles findRe from findInput and updates findErr.
func (m *Model) recompileFindRe() {
	if m.findInput == "" {
		m.findRe = nil
		m.findErr = ""
		return
	}
	re, err := regexp.Compile(m.findInput)
	if err != nil {
		m.findRe = nil
		m.findErr = err.Error()
		return
	}
	m.findRe = re
	m.findErr = ""
}

// matchCount returns the number of directory entries matching the current regex.
func (m *Model) matchCount() int {
	if m.findRe == nil {
		return 0
	}
	n := 0
	for _, e := range m.entries {
		if m.findRe.MatchString(e.Name()) {
			n++
		}
	}
	return n
}

// buildChanges computes m.changes from the current find/replace state.
func (m *Model) buildChanges() {
	if m.findRe == nil {
		m.changes = nil
		return
	}
	var changes []changeEntry
	counter := 1
	for _, e := range m.entries {
		name := e.Name()
		if !m.findRe.MatchString(name) {
			continue
		}
		if m.op == opDelete {
			changes = append(changes, changeEntry{from: name, isDir: e.IsDir()})
			continue
		}
		repl := substituteCounter(m.replaceInput, counter)
		newName := m.findRe.ReplaceAllString(name, repl)
		counter++
		if newName == name || newName == "" {
			continue // effectively unchanged
		}
		changes = append(changes, changeEntry{from: name, to: newName, isDir: e.IsDir()})
	}
	m.changes = changes
}

// previewChanges computes rename previews for live display without storing them.
// For delete mode it returns entries with empty to field.
func (m *Model) previewChanges() []changeEntry {
	if m.findRe == nil {
		return nil
	}
	var changes []changeEntry
	counter := 1
	for _, e := range m.entries {
		name := e.Name()
		if !m.findRe.MatchString(name) {
			continue
		}
		if m.op == opDelete {
			changes = append(changes, changeEntry{from: name, isDir: e.IsDir()})
			continue
		}
		repl := substituteCounter(m.replaceInput, counter)
		newName := m.findRe.ReplaceAllString(name, repl)
		counter++
		changes = append(changes, changeEntry{from: name, to: newName, isDir: e.IsDir()})
	}
	return changes
}

// applyDeletes removes all entries in m.changes from disk.
func (m *Model) applyDeletes() {
	for _, c := range m.changes {
		path := filepath.Join(m.dir, c.from)
		if err := os.RemoveAll(path); err != nil {
			m.results = append(m.results, styleError.Render(fmt.Sprintf("  delete %s: %v", c.from, err)))
		} else {
			m.results = append(m.results, styleSuccess.Render(fmt.Sprintf("  deleted  %s", c.from)))
		}
	}
}

// applyRenames performs the three-step safe rename:
//  1. Build from→temp and temp→to maps.
//  2. Rename every source to a unique temporary name (avoids collisions).
//  3. Rename every temporary name to the final target.
func (m *Model) applyRenames() {
	type triple struct{ from, temp, to string }
	triples := make([]triple, len(m.changes))
	for i, c := range m.changes {
		triples[i] = triple{c.from, "__babi_sr_" + randHex(8), c.to}
	}

	// Step 2: from → temp
	for _, t := range triples {
		if err := os.Rename(filepath.Join(m.dir, t.from), filepath.Join(m.dir, t.temp)); err != nil {
			m.results = append(m.results, styleError.Render(fmt.Sprintf("  rename %s→temp: %v", t.from, err)))
		}
	}

	// Step 3: temp → to
	for _, t := range triples {
		if err := os.Rename(filepath.Join(m.dir, t.temp), filepath.Join(m.dir, t.to)); err != nil {
			m.results = append(m.results, styleError.Render(fmt.Sprintf("  rename temp→%s: %v", t.to, err)))
		} else {
			m.results = append(m.results, styleSuccess.Render(fmt.Sprintf("  %s  →  %s", t.from, t.to)))
		}
	}
}

func randHex(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

// ---------------------------------------------------------------------------
// tea.Model
// ---------------------------------------------------------------------------

// Init satisfies tea.Model; no initial commands needed.
func (m Model) Init() tea.Cmd { return nil }

// Update dispatches messages to the active screen handler.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil
	case tea.KeyMsg:
		switch m.screen {
		case screenPattern:
			return m.handlePatternKey(msg)
		case screenReplace:
			return m.handleReplaceKey(msg)
		case screenConfirm:
			return m.handleConfirmKey(msg)
		case screenDone:
			return m.handleDoneKey(msg)
		}
	}
	return m, nil
}

func (m Model) handlePatternKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit

	case "backspace":
		runes := []rune(m.findInput)
		if len(runes) > 0 {
			m.findInput = string(runes[:len(runes)-1])
			m.recompileFindRe()
			m.scroll = 0
		}

	case "down":
		m.scroll++

	case "up":
		if m.scroll > 0 {
			m.scroll--
		}

	case "ctrl+r":
		if m.findRe != nil && m.matchCount() > 0 {
			m.op = opRename
			m.screen = screenReplace
			m.scroll = 0
		}

	case "ctrl+d":
		if m.findRe != nil && m.matchCount() > 0 {
			m.op = opDelete
			m.buildChanges()
			m.screen = screenConfirm
			m.scroll = 0
		}

	default:
		if m.ready && len(msg.Runes) > 0 {
			m.findInput += string(msg.Runes)
			m.recompileFindRe()
			m.scroll = 0
		}
	}
	return m, nil
}

func (m Model) handleReplaceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.screen = screenPattern
		m.replaceInput = ""
		m.scroll = 0

	case "backspace":
		runes := []rune(m.replaceInput)
		if len(runes) > 0 {
			m.replaceInput = string(runes[:len(runes)-1])
		}

	case "enter":
		m.buildChanges()
		m.screen = screenConfirm
		m.scroll = 0

	case "down":
		m.scroll++

	case "up":
		if m.scroll > 0 {
			m.scroll--
		}

	default:
		if len(msg.Runes) > 0 {
			m.replaceInput += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc", "n", "N":
		m.scroll = 0
		if m.op == opDelete {
			m.screen = screenPattern
		} else {
			m.screen = screenReplace
		}

	case "y", "Y":
		m.results = nil
		if m.op == opDelete {
			m.applyDeletes()
		} else {
			m.applyRenames()
		}
		m.screen = screenDone
		m.scroll = 0

	case "down":
		m.scroll++

	case "up":
		if m.scroll > 0 {
			m.scroll--
		}
	}
	return m, nil
}

func (m Model) handleDoneKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c", "enter":
		return m, tea.Quit
	case "down":
		m.scroll++
	case "up":
		if m.scroll > 0 {
			m.scroll--
		}
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the full TUI for the current screen.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	switch m.screen {
	case screenPattern:
		return m.viewPattern()
	case screenReplace:
		return m.viewReplace()
	case screenConfirm:
		return m.viewConfirm()
	case screenDone:
		return m.viewDone()
	}
	return ""
}

// viewPattern renders the find-pattern input screen with a live entry list.
func (m Model) viewPattern() string {
	header := renderHeader(m.width, "babi sr", truncate(m.dir, m.width/2))

	// Reserve: header(1) + input(1) + info(1) + footer(1) = 4
	listH := m.height - 4
	if listH < 0 {
		listH = 0
	}

	inputLine := styleInputLabel.Render("Find:  ") + m.findInput + "█"

	var infoLine string
	switch {
	case m.findErr != "":
		infoLine = styleError.Render("  " + truncate(m.findErr, m.width-4))
	case m.findRe != nil:
		n := m.matchCount()
		if n == 0 {
			infoLine = styleDimRow.Render("  no matches")
		} else {
			noun := "match"
			if n != 1 {
				noun = "matches"
			}
			infoLine = styleSuccess.Render(fmt.Sprintf("  %d %s", n, noun))
		}
	default:
		infoLine = styleDimRow.Render("  type a regex to filter entries")
	}

	entryLines := m.buildEntryLines()
	visible := scrollSlice(entryLines, m.scroll, listH)

	var footer string
	if m.findRe != nil && m.matchCount() > 0 {
		footer = renderFooter(m.width, "^R", "rename", "^D", "delete", "↑/↓", "scroll", "esc", "quit")
	} else {
		footer = renderFooter(m.width, "↑/↓", "scroll", "esc", "quit")
	}

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteByte('\n')
	sb.WriteString(inputLine)
	sb.WriteByte('\n')
	sb.WriteString(infoLine)
	sb.WriteByte('\n')
	sb.WriteString(strings.Join(visible, "\n"))
	sb.WriteByte('\n')
	sb.WriteString(footer)
	return sb.String()
}

// buildEntryLines renders one line per directory entry with match highlighting.
func (m Model) buildEntryLines() []string {
	lines := make([]string, 0, len(m.entries))
	for _, e := range m.entries {
		name := e.Name()
		matched := m.findRe != nil && m.findRe.MatchString(name)
		var line string
		switch {
		case matched && e.IsDir():
			line = styleDirMatch.Render("  ✓ " + name + "/")
		case matched:
			line = styleFileMatch.Render("  ✓ " + name)
		case e.IsDir():
			line = styleDimRow.Render("    " + name + "/")
		default:
			line = styleDimRow.Render("    " + name)
		}
		lines = append(lines, line)
	}
	return lines
}

// viewReplace renders the replacement-pattern input with a live rename preview.
func (m Model) viewReplace() string {
	header := renderHeader(m.width, "babi sr", truncate(m.dir, m.width/2))

	// Reserve: header(1) + find(1) + replace(1) + hint(1) + footer(1) = 5
	listH := m.height - 5
	if listH < 0 {
		listH = 0
	}

	findLine := styleInputLabel.Render("Find:    ") + styleDimRow.Render(m.findInput)
	replLine := styleInputLabel.Render("Replace: ") + m.replaceInput + "█"
	hintLine := styleDimRow.Render("  $1 $2… for captures · {#} or {#:3} for counter")

	previewLines := m.buildRenamePreviewLines()
	visible := scrollSlice(previewLines, m.scroll, listH)

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteByte('\n')
	sb.WriteString(findLine)
	sb.WriteByte('\n')
	sb.WriteString(replLine)
	sb.WriteByte('\n')
	sb.WriteString(hintLine)
	sb.WriteByte('\n')
	sb.WriteString(strings.Join(visible, "\n"))
	sb.WriteByte('\n')
	sb.WriteString(renderFooter(m.width, "enter", "confirm", "↑/↓", "scroll", "esc", "back"))
	return sb.String()
}

// buildRenamePreviewLines shows all entries; matched ones show from→to.
func (m Model) buildRenamePreviewLines() []string {
	changes := m.previewChanges()

	// Index changes by from-name for O(1) lookup.
	type preview struct{ to string }
	byName := make(map[string]preview, len(changes))
	for _, c := range changes {
		byName[c.from] = preview{c.to}
	}

	arrow := styleArrow.Render("  →  ")
	lines := make([]string, 0, len(m.entries))

	for _, e := range m.entries {
		name := e.Name()
		p, matched := byName[name]
		if !matched {
			if e.IsDir() {
				lines = append(lines, styleUnchanged.Render("     "+name+"/"))
			} else {
				lines = append(lines, styleUnchanged.Render("     "+name))
			}
			continue
		}

		fromStr := name
		if e.IsDir() {
			fromStr += "/"
		}
		toStr := p.to
		if e.IsDir() && toStr != "" {
			toStr += "/"
		}

		var fromRendered string
		if e.IsDir() {
			fromRendered = styleDirMatch.Render(fromStr)
		} else {
			fromRendered = styleFileMatch.Render(fromStr)
		}

		if p.to == "" || p.to == name {
			lines = append(lines, styleUnchanged.Render("  = "+fromStr))
		} else {
			lines = append(lines, "  "+fromRendered+arrow+styleToName.Render(toStr))
		}
	}
	return lines
}

// viewConfirm renders the confirmation screen listing all pending changes.
func (m Model) viewConfirm() string {
	header := renderHeader(m.width, "babi sr", truncate(m.dir, m.width/2))

	// Reserve: header(1) + title(1) + footer(1) = 3
	listH := m.height - 3
	if listH < 0 {
		listH = 0
	}

	var titleLine string
	if m.op == opDelete {
		titleLine = styleError.Render(fmt.Sprintf("  Delete %d item(s) — confirm?", len(m.changes)))
	} else {
		titleLine = styleSuccess.Render(fmt.Sprintf("  Rename %d item(s) — confirm?", len(m.changes)))
	}

	changeLines := m.buildChangeLines()
	if len(changeLines) == 0 {
		changeLines = []string{styleDimRow.Render("  (no effective changes)")}
	}
	visible := scrollSlice(changeLines, m.scroll, listH)

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteByte('\n')
	sb.WriteString(titleLine)
	sb.WriteByte('\n')
	sb.WriteString(strings.Join(visible, "\n"))
	sb.WriteByte('\n')
	sb.WriteString(renderFooter(m.width, "y", "apply", "n/esc", "back", "↑/↓", "scroll"))
	return sb.String()
}

// buildChangeLines returns one rendered line per changeEntry.
func (m Model) buildChangeLines() []string {
	arrow := styleArrow.Render("  →  ")
	lines := make([]string, 0, len(m.changes))
	for _, c := range m.changes {
		fromStr := c.from
		if c.isDir {
			fromStr += "/"
		}
		if m.op == opDelete {
			lines = append(lines, styleDelMark.Render("  ✗ ")+styleError.Render(fromStr))
			continue
		}
		toStr := c.to
		if c.isDir {
			toStr += "/"
		}
		var fromRendered string
		if c.isDir {
			fromRendered = styleDirMatch.Render(fromStr)
		} else {
			fromRendered = styleFileMatch.Render(fromStr)
		}
		lines = append(lines, "  "+fromRendered+arrow+styleToName.Render(toStr))
	}
	return lines
}

// viewDone renders the results screen after operations complete.
func (m Model) viewDone() string {
	header := renderHeader(m.width, "babi sr — done", truncate(m.dir, m.width/2))

	// Reserve: header(1) + footer(1) = 2
	listH := m.height - 2
	if listH < 0 {
		listH = 0
	}

	lines := m.results
	if len(lines) == 0 {
		lines = []string{styleDimRow.Render("  (nothing done)")}
	}
	visible := scrollSlice(lines, m.scroll, listH)

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteByte('\n')
	sb.WriteString(strings.Join(visible, "\n"))
	sb.WriteByte('\n')
	sb.WriteString(renderFooter(m.width, "↑/↓", "scroll", "q", "quit"))
	return sb.String()
}

// ---------------------------------------------------------------------------
// Scroll helper
// ---------------------------------------------------------------------------

// scrollSlice returns height lines from lines starting at offset scroll,
// padding with empty strings if the slice is shorter than height.
func scrollSlice(lines []string, scroll, height int) []string {
	total := len(lines)
	start := scroll
	if start > total {
		start = total
	}
	end := start + height
	if end > total {
		end = total
	}
	visible := make([]string, height)
	copy(visible, lines[start:end])
	return visible
}
