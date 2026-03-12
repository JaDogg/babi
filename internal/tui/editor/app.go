package editor

import (
	"errors"
	"fmt"
	"os"
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
	colorMuted   = lipgloss.AdaptiveColor{Light: "#767676", Dark: "#626262"}
	colorFaint   = lipgloss.AdaptiveColor{Light: "#DEDEDE", Dark: "#333333"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#02A699", Dark: "#04D18A"}
	colorError   = lipgloss.AdaptiveColor{Light: "#CC3333", Dark: "#FF5555"}

	styleHeaderBg    = lipgloss.NewStyle().Background(colorPrimary)
	styleHeaderTitle = lipgloss.NewStyle().Background(colorPrimary).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 2)
	styleHeaderRight = lipgloss.NewStyle().Background(colorPrimary).Foreground(lipgloss.Color("#CCC8FF")).Padding(0, 2)
	styleFooterBar   = lipgloss.NewStyle().Background(colorFaint).Foreground(colorMuted).Padding(0, 2)
	styleFooterKey   = lipgloss.NewStyle().Background(colorMuted).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1)
	styleLineNum     = lipgloss.NewStyle().Foreground(colorMuted)
	styleGutter      = lipgloss.NewStyle().Foreground(colorFaint)
	styleCursorLine  = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "#F0EEFF", Dark: "#1E1A2E"})
	styleSuccess     = lipgloss.NewStyle().Foreground(colorSuccess)
	styleError       = lipgloss.NewStyle().Foreground(colorError)
	styleCursor      = lipgloss.NewStyle().Reverse(true)
)

// ---------------------------------------------------------------------------
// Layout helpers
// ---------------------------------------------------------------------------

func renderHeader(width int, title, right string) string {
	titleStr := styleHeaderTitle.Render(title)
	rightStr := styleHeaderRight.Render(right)

	titleWidth := lipgloss.Width(titleStr)
	rightWidth := lipgloss.Width(rightStr)
	gap := width - titleWidth - rightWidth
	if gap < 0 {
		gap = 0
	}

	filler := styleHeaderBg.Render(strings.Repeat(" ", gap))
	return lipgloss.JoinHorizontal(lipgloss.Top, titleStr, filler, rightStr)
}

func renderFooter(width int, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		key := styleFooterKey.Render(pairs[i])
		desc := styleFooterBar.Render(" " + pairs[i+1])
		parts = append(parts, key+desc)
	}
	content := strings.Join(parts, styleFooterBar.Render("  "))
	contentWidth := lipgloss.Width(content)
	padding := width - contentWidth
	if padding < 0 {
		padding = 0
	}
	return content + styleFooterBar.Render(strings.Repeat(" ", padding))
}

func padToHeight(s string, height int) string {
	lines := strings.Split(s, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Model types
// ---------------------------------------------------------------------------

type editorMode int

const (
	modeEdit editorMode = iota
	modeConfirmQuit
)

// Model is the bubbletea model for the editor.
type Model struct {
	filepath string
	lines    []string
	curRow   int
	curCol   int
	topRow   int
	leftCol  int
	modified bool
	mode     editorMode
	status   string
	width    int
	height   int
}

// New creates a Model by reading the file at filepath.
// If the file does not exist, an empty buffer is returned (no error).
func New(filepath string) (Model, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			m := NewEmpty()
			m.filepath = filepath
			return m, nil
		}
		return Model{}, err
	}

	content := string(data)
	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimRight(content, "\n")

	var lines []string
	if content == "" {
		lines = []string{""}
	} else {
		lines = strings.Split(content, "\n")
	}

	return Model{
		filepath: filepath,
		lines:    lines,
		width:    80,
		height:   24,
	}, nil
}

// NewEmpty creates a Model with an empty buffer and no associated file.
func NewEmpty() Model {
	return Model{
		lines:  []string{""},
		width:  80,
		height: 24,
	}
}

// ---------------------------------------------------------------------------
// tea.Model interface
// ---------------------------------------------------------------------------

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	// Header
	title := "babi edit"
	if m.filepath != "" {
		title += " — " + m.filepath
	}
	if m.modified {
		title += "*"
	}
	pos := fmt.Sprintf("%d:%d", m.curRow+1, m.curCol+1)
	header := renderHeader(m.width, title, pos)

	// Footer
	var footer string
	if m.mode == modeConfirmQuit {
		footer = styleFooterBar.Render(
			lipgloss.NewStyle().Width(m.width - 4).Render("Unsaved changes. Quit anyway? (y/n)"),
		)
	} else if m.status != "" {
		footer = styleFooterBar.Render(
			lipgloss.NewStyle().Width(m.width - 4).Render(m.status),
		)
	} else {
		footer = renderFooter(m.width, "ctrl+s", "save", "ctrl+q", "quit", "arrows", "navigate")
	}

	// Content area
	contentHeight := m.height - 2 // minus header and footer

	gutterWidth := m.gutterWidth()
	contentWidth := m.width - gutterWidth
	if contentWidth < 1 {
		contentWidth = 1
	}

	var sb strings.Builder
	for i := 0; i < contentHeight; i++ {
		row := m.topRow + i
		if i > 0 {
			sb.WriteByte('\n')
		}

		if row >= len(m.lines) {
			// Empty line beyond content
			lineNumStr := styleLineNum.Render(strings.Repeat(" ", gutterWidth-3))
			gutterSep := styleGutter.Render(" │ ")
			line := lineNumStr + gutterSep
			if row == m.curRow {
				line = styleCursorLine.Render(strings.Repeat(" ", m.width))
			}
			sb.WriteString(line)
			continue
		}

		// Render line number
		totalLines := len(m.lines)
		numWidth := len(fmt.Sprintf("%d", totalLines))
		lineNumStr := styleLineNum.Render(fmt.Sprintf("%*d", numWidth, row+1))
		gutterSep := styleGutter.Render(" │ ")

		// Get line content, apply horizontal scroll
		lineRunes := []rune(m.lines[row])
		visibleRunes := lineRunes
		if m.leftCol < len(lineRunes) {
			visibleRunes = lineRunes[m.leftCol:]
		} else {
			visibleRunes = []rune{}
		}

		// Truncate to content width
		if len(visibleRunes) > contentWidth {
			visibleRunes = visibleRunes[:contentWidth]
		}

		var lineContent string
		if row == m.curRow {
			// Render cursor on this line
			cursorInView := m.curCol - m.leftCol
			lineContent = m.renderCursorLine(visibleRunes, cursorInView, contentWidth)
			fullLine := lineNumStr + gutterSep + lineContent
			// Pad to full width with cursor line background
			rendered := lipgloss.Width(fullLine)
			if rendered < m.width {
				fullLine += styleCursorLine.Render(strings.Repeat(" ", m.width-rendered))
			}
			sb.WriteString(fullLine)
		} else {
			lineContent = string(visibleRunes)
			sb.WriteString(lineNumStr + gutterSep + lineContent)
		}
	}

	content := sb.String()

	// Status message overlay (shown in status area - just keep in footer)
	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clear status on any keypress
	m.status = ""

	if m.mode == modeConfirmQuit {
		switch msg.String() {
		case "y", "Y":
			return m, tea.Quit
		default:
			m.mode = modeEdit
		}
		return m, nil
	}

	switch msg.String() {
	// Movement
	case "up":
		if m.curRow > 0 {
			m.curRow--
			m.clampCurCol()
		}
	case "down":
		if m.curRow < len(m.lines)-1 {
			m.curRow++
			m.clampCurCol()
		}
	case "left":
		if m.curCol > 0 {
			m.curCol--
		} else if m.curRow > 0 {
			m.curRow--
			m.curCol = utf8.RuneCountInString(m.lines[m.curRow])
		}
	case "right":
		lineLen := utf8.RuneCountInString(m.lines[m.curRow])
		if m.curCol < lineLen {
			m.curCol++
		} else if m.curRow < len(m.lines)-1 {
			m.curRow++
			m.curCol = 0
		}
	case "home":
		m.curCol = 0
	case "end":
		m.curCol = utf8.RuneCountInString(m.lines[m.curRow])
	case "ctrl+home":
		m.curRow = 0
		m.curCol = 0
	case "ctrl+end":
		m.curRow = len(m.lines) - 1
		m.curCol = utf8.RuneCountInString(m.lines[m.curRow])
	case "pgup":
		contentHeight := m.height - 2
		m.curRow -= contentHeight
		if m.curRow < 0 {
			m.curRow = 0
		}
		m.clampCurCol()
	case "pgdown":
		contentHeight := m.height - 2
		m.curRow += contentHeight
		if m.curRow >= len(m.lines) {
			m.curRow = len(m.lines) - 1
		}
		m.clampCurCol()

	// Editing
	case "enter":
		m.insertNewline()
	case "backspace":
		m.backspace()
	case "delete":
		m.deleteRight()
	case "tab":
		m.insertRunes([]rune("    "))

	// Save
	case "ctrl+s":
		if err := m.save(); err != nil {
			m.status = styleError.Render("Error: " + err.Error())
		} else {
			m.status = styleSuccess.Render("Saved")
			m.modified = false
		}

	// Quit
	case "ctrl+q":
		if m.modified {
			m.mode = modeConfirmQuit
		} else {
			return m, tea.Quit
		}

	default:
		// Printable characters
		if msg.Type == tea.KeyRunes {
			m.insertRunes(msg.Runes)
		}
	}

	m.scrollToCursor()
	return m, nil
}

// ---------------------------------------------------------------------------
// Editing helpers
// ---------------------------------------------------------------------------

func (m *Model) insertRunes(runes []rune) {
	row := m.curRow
	line := []rune(m.lines[row])
	col := m.curCol
	if col > len(line) {
		col = len(line)
	}
	newLine := make([]rune, 0, len(line)+len(runes))
	newLine = append(newLine, line[:col]...)
	newLine = append(newLine, runes...)
	newLine = append(newLine, line[col:]...)
	m.lines[row] = string(newLine)
	m.curCol = col + len(runes)
	m.modified = true
}

func (m *Model) insertNewline() {
	row := m.curRow
	line := []rune(m.lines[row])
	col := m.curCol
	if col > len(line) {
		col = len(line)
	}
	before := string(line[:col])
	after := string(line[col:])
	m.lines[row] = before
	newLines := make([]string, 0, len(m.lines)+1)
	newLines = append(newLines, m.lines[:row+1]...)
	newLines = append(newLines, after)
	newLines = append(newLines, m.lines[row+1:]...)
	m.lines = newLines
	m.curRow++
	m.curCol = 0
	m.modified = true
}

func (m *Model) backspace() {
	row := m.curRow
	col := m.curCol
	if col > 0 {
		line := []rune(m.lines[row])
		if col > len(line) {
			col = len(line)
		}
		newLine := make([]rune, 0, len(line)-1)
		newLine = append(newLine, line[:col-1]...)
		newLine = append(newLine, line[col:]...)
		m.lines[row] = string(newLine)
		m.curCol = col - 1
		m.modified = true
	} else if row > 0 {
		// Merge with previous line
		prevLine := m.lines[row-1]
		prevLen := utf8.RuneCountInString(prevLine)
		m.lines[row-1] = prevLine + m.lines[row]
		newLines := make([]string, 0, len(m.lines)-1)
		newLines = append(newLines, m.lines[:row]...)
		newLines = append(newLines, m.lines[row+1:]...)
		m.lines = newLines
		m.curRow--
		m.curCol = prevLen
		m.modified = true
	}
}

func (m *Model) deleteRight() {
	row := m.curRow
	col := m.curCol
	line := []rune(m.lines[row])
	if col < len(line) {
		newLine := make([]rune, 0, len(line)-1)
		newLine = append(newLine, line[:col]...)
		newLine = append(newLine, line[col+1:]...)
		m.lines[row] = string(newLine)
		m.modified = true
	} else if row < len(m.lines)-1 {
		// Merge next line into current
		m.lines[row] = m.lines[row] + m.lines[row+1]
		newLines := make([]string, 0, len(m.lines)-1)
		newLines = append(newLines, m.lines[:row+1]...)
		newLines = append(newLines, m.lines[row+2:]...)
		m.lines = newLines
		m.modified = true
	}
}

func (m *Model) clampCurCol() {
	lineLen := utf8.RuneCountInString(m.lines[m.curRow])
	if m.curCol > lineLen {
		m.curCol = lineLen
	}
}

// ---------------------------------------------------------------------------
// Scroll helpers
// ---------------------------------------------------------------------------

func (m *Model) scrollToCursor() {
	contentHeight := m.height - 2
	context := 2

	// Vertical scroll
	if m.curRow < m.topRow+context {
		m.topRow = m.curRow - context
	}
	if m.curRow >= m.topRow+contentHeight-context {
		m.topRow = m.curRow - contentHeight + context + 1
	}
	if m.topRow < 0 {
		m.topRow = 0
	}

	// Horizontal scroll
	gutterWidth := m.gutterWidth()
	contentWidth := m.width - gutterWidth
	if contentWidth < 1 {
		contentWidth = 1
	}

	if m.curCol < m.leftCol {
		m.leftCol = m.curCol
	}
	if m.curCol >= m.leftCol+contentWidth {
		m.leftCol = m.curCol - contentWidth + 1
	}
	if m.leftCol < 0 {
		m.leftCol = 0
	}
}

func (m *Model) gutterWidth() int {
	numWidth := len(fmt.Sprintf("%d", len(m.lines)))
	// numWidth digits + " │ " (3 chars)
	return numWidth + 3
}

// ---------------------------------------------------------------------------
// Cursor line rendering
// ---------------------------------------------------------------------------

func (m *Model) renderCursorLine(visibleRunes []rune, cursorInView, contentWidth int) string {
	var sb strings.Builder

	if cursorInView < 0 || cursorInView > len(visibleRunes) {
		// Cursor not visible horizontally, just render plain text with cursor line bg
		text := string(visibleRunes)
		return styleCursorLine.Render(text)
	}

	// Before cursor
	if cursorInView > 0 {
		before := string(visibleRunes[:cursorInView])
		sb.WriteString(styleCursorLine.Render(before))
	}

	// Cursor character
	if cursorInView < len(visibleRunes) {
		ch := string(visibleRunes[cursorInView : cursorInView+1])
		sb.WriteString(styleCursor.Render(ch))
		// After cursor
		if cursorInView+1 < len(visibleRunes) {
			after := string(visibleRunes[cursorInView+1:])
			sb.WriteString(styleCursorLine.Render(after))
		}
	} else {
		// Cursor at end of line — show block cursor on a space
		sb.WriteString(styleCursor.Render(" "))
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// Save
// ---------------------------------------------------------------------------

func (m *Model) save() error {
	if m.filepath == "" {
		return fmt.Errorf("no filepath set")
	}
	content := strings.Join(m.lines, "\n")
	if len(m.lines) > 0 && m.lines[len(m.lines)-1] != "" {
		content += "\n"
	}
	return os.WriteFile(m.filepath, []byte(content), 0o644)
}

// ---------------------------------------------------------------------------
// Accessors (useful for embedding)
// ---------------------------------------------------------------------------

// Lines returns the current buffer lines.
func (m Model) Lines() []string {
	return m.lines
}

// Filepath returns the associated file path.
func (m Model) Filepath() string {
	return m.filepath
}

// Modified reports whether the buffer has unsaved changes.
func (m Model) Modified() bool {
	return m.modified
}

// SetFilepath sets the file path for save operations.
func (m *Model) SetFilepath(fp string) {
	m.filepath = fp
}

// suppress unused variable warnings for styleSuccess/styleError used only via Render
var _ = styleSuccess
var _ = styleError
