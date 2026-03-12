package hex

import (
	"fmt"
	"os"
	"strings"
	"unicode"

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
	styleOffset      = lipgloss.NewStyle().Foreground(colorMuted)
	styleCursorByte  = lipgloss.NewStyle().Background(colorPrimary).Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	styleModified    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#D4A800", Dark: "#FFCC00"})
	styleSuccess     = lipgloss.NewStyle().Foreground(colorSuccess)
	styleError       = lipgloss.NewStyle().Foreground(colorError)
)

// ---------------------------------------------------------------------------
// Layout helpers
// ---------------------------------------------------------------------------

// renderHeader renders a full-width header bar: title left, info right.
func renderHeader(width int, title, right string) string {
	l := styleHeaderTitle.Render(" " + title)
	r := styleHeaderRight.Render(right + " ")
	gap := width - lipgloss.Width(l) - lipgloss.Width(r)
	if gap < 0 {
		gap = 0
	}
	fill := styleHeaderBg.Render(strings.Repeat(" ", gap))
	return l + fill + r
}

// renderFooter renders a full-width footer bar from key/desc pairs.
// pairs alternates: key, description, key, description, ...
func renderFooter(width int, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		k := styleFooterKey.Render(pairs[i])
		parts = append(parts, k+"  "+pairs[i+1])
	}
	content := strings.Join(parts, "   ")
	return styleFooterBar.Width(width - 4).Render(content)
}

// padToHeight ensures s contains exactly height visual lines (no trailing newline).
func padToHeight(s string, height int) string {
	s = strings.TrimRight(s, "\n")
	n := strings.Count(s, "\n") + 1
	if n < height {
		s += strings.Repeat("\n", height-n)
	}
	return s
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

const bytesPerRow = 16

// Model is the bubbletea model for the hex editor.
type Model struct {
	filepath string
	data     []byte
	cursor   int  // current byte offset
	topRow   int  // first visible row (in 16-byte rows)
	editMode bool
	nibble   int  // 0=high nibble, 1=low nibble (only in editMode)
	modified bool
	status   string // transient message
	width    int
	height   int
}

// New reads the file at filepath into memory and returns a ready-to-use Model.
func New(filepath string) (Model, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return Model{}, fmt.Errorf("hex: read %q: %w", filepath, err)
	}
	return Model{
		filepath: filepath,
		data:     data,
	}, nil
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
		m.clampScroll()

	case tea.KeyMsg:
		// Clear transient status on any key press.
		m.status = ""

		if m.editMode {
			return m.updateEditMode(msg)
		}
		return m.updateNavMode(msg)
	}

	return m, nil
}

// updateNavMode handles key events when not in edit mode.
func (m Model) updateNavMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	visibleRows := m.visibleRows()

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		if m.cursor+bytesPerRow < len(m.data) {
			m.cursor += bytesPerRow
		} else {
			// Move to last byte if we can't go a full row down.
			m.cursor = len(m.data) - 1
			if m.cursor < 0 {
				m.cursor = 0
			}
		}

	case "k", "up":
		if m.cursor >= bytesPerRow {
			m.cursor -= bytesPerRow
		} else {
			m.cursor = 0
		}

	case "l", "right":
		if m.cursor < len(m.data)-1 {
			m.cursor++
		}

	case "h", "left":
		if m.cursor > 0 {
			m.cursor--
		}

	case "pgdown", "ctrl+f":
		m.cursor += visibleRows * bytesPerRow
		if m.cursor >= len(m.data) {
			m.cursor = len(m.data) - 1
			if m.cursor < 0 {
				m.cursor = 0
			}
		}

	case "pgup", "ctrl+b":
		m.cursor -= visibleRows * bytesPerRow
		if m.cursor < 0 {
			m.cursor = 0
		}

	case "g":
		m.cursor = 0

	case "G":
		if len(m.data) > 0 {
			m.cursor = len(m.data) - 1
		}

	case "e", "i":
		m.editMode = true
		m.nibble = 0
	}

	m.clampScroll()
	return m, nil
}

// updateEditMode handles key events when in edit mode.
func (m Model) updateEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editMode = false
		m.nibble = 0

	case "ctrl+c":
		return m, tea.Quit

	case "ctrl+s":
		err := os.WriteFile(m.filepath, m.data, 0o644)
		if err != nil {
			m.status = styleError.Render("save failed: " + err.Error())
		} else {
			m.modified = false
			m.status = styleSuccess.Render("saved " + m.filepath)
		}

	case "backspace":
		if m.nibble == 1 {
			// Reset high nibble editing — stay on same byte.
			m.nibble = 0
		} else if m.cursor > 0 {
			m.cursor--
			m.nibble = 0
		}

	default:
		k := msg.String()
		if len(k) == 1 {
			ch := rune(k[0])
			var nibVal byte
			switch {
			case ch >= '0' && ch <= '9':
				nibVal = byte(ch - '0')
			case ch >= 'a' && ch <= 'f':
				nibVal = byte(ch-'a') + 10
			case ch >= 'A' && ch <= 'F':
				nibVal = byte(ch-'A') + 10
			default:
				// Not a hex digit — ignore.
				m.clampScroll()
				return m, nil
			}

			if len(m.data) == 0 {
				m.clampScroll()
				return m, nil
			}

			if m.nibble == 0 {
				// High nibble: replace upper 4 bits, keep lower 4.
				m.data[m.cursor] = (nibVal << 4) | (m.data[m.cursor] & 0x0F)
				m.nibble = 1
				m.modified = true
			} else {
				// Low nibble: keep upper 4 bits, replace lower 4.
				m.data[m.cursor] = (m.data[m.cursor] & 0xF0) | nibVal
				m.nibble = 0
				m.modified = true
				// Advance cursor.
				if m.cursor < len(m.data)-1 {
					m.cursor++
				}
			}
		}
	}

	m.clampScroll()
	return m, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	// --- Header ---
	title := "babi hex — " + m.filepath
	modeStr := ""
	if m.editMode {
		modeStr = "[EDIT]  "
	}
	right := modeStr + fmt.Sprintf("%d bytes", len(m.data))
	header := renderHeader(m.width, title, right)

	// --- Footer ---
	var footer string
	if m.editMode {
		footer = renderFooter(m.width,
			"0-9 a-f", "edit byte",
			"⌫", "back",
			"ctrl+s", "save",
			"esc", "exit edit",
		)
	} else {
		footer = renderFooter(m.width,
			"h j k l", "navigate",
			"g G", "start/end",
			"e", "edit mode",
			"q", "quit",
		)
	}

	// --- Status line ---
	statusLine := ""
	if m.status != "" {
		statusLine = m.status
	} else if m.modified {
		statusLine = styleModified.Render("  [modified — ctrl+s to save]")
	}

	// --- Content ---
	headerLines := 1
	footerLines := 1
	statusLines := 1
	contentHeight := m.height - headerLines - footerLines - statusLines
	if contentHeight < 1 {
		contentHeight = 1
	}

	content := m.renderHex(contentHeight)

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteByte('\n')
	sb.WriteString(padToHeight(content, contentHeight))
	sb.WriteByte('\n')
	sb.WriteString(statusLine)
	sb.WriteByte('\n')
	sb.WriteString(footer)

	return sb.String()
}

// renderHex renders the hex dump for the visible rows.
func (m Model) renderHex(visibleH int) string {
	if len(m.data) == 0 {
		return styleOffset.Render("  (empty file)")
	}

	cursorRow := m.cursor / bytesPerRow
	var lines []string

	for row := m.topRow; row < m.topRow+visibleH; row++ {
		offset := row * bytesPerRow
		if offset >= len(m.data) {
			lines = append(lines, "")
			continue
		}

		end := offset + bytesPerRow
		if end > len(m.data) {
			end = len(m.data)
		}
		rowBytes := m.data[offset:end]

		// Offset column.
		offsetStr := styleOffset.Render(fmt.Sprintf("%08x  ", offset))

		// Hex columns: 4 groups of 4 bytes.
		hexPart := m.renderHexBytes(rowBytes, offset)

		// ASCII column.
		asciiPart := m.renderASCII(rowBytes, offset)

		// Highlight entire cursor row background subtly.
		line := offsetStr + hexPart + "  |" + asciiPart + "|"
		if row == cursorRow && !m.editMode {
			// Dim row highlight is already implied by cursor byte highlight.
		}

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// renderHexBytes renders the hex byte columns for one row.
func (m Model) renderHexBytes(rowBytes []byte, rowOffset int) string {
	var sb strings.Builder

	for i := 0; i < bytesPerRow; i++ {
		// Space between groups of 4.
		if i > 0 && i%4 == 0 {
			sb.WriteByte(' ')
		}
		// Space between individual bytes.
		if i > 0 {
			sb.WriteByte(' ')
		}

		byteOffset := rowOffset + i
		if i >= len(rowBytes) {
			// Padding for incomplete last row.
			sb.WriteString("  ")
			continue
		}

		b := rowBytes[i]
		hexStr := fmt.Sprintf("%02x", b)

		if byteOffset == m.cursor {
			if m.editMode && m.nibble == 1 {
				// High nibble already typed — show what was set, highlight low nibble.
				hi := string(hexStr[0])
				lo := string(hexStr[1])
				sb.WriteString(hi)
				sb.WriteString(styleCursorByte.Render(lo))
			} else {
				sb.WriteString(styleCursorByte.Render(hexStr))
			}
		} else {
			sb.WriteString(hexStr)
		}
	}

	return sb.String()
}

// renderASCII renders the ASCII column for one row.
func (m Model) renderASCII(rowBytes []byte, rowOffset int) string {
	var sb strings.Builder

	for i, b := range rowBytes {
		byteOffset := rowOffset + i
		var ch string
		if b >= 0x20 && b < 0x7f && unicode.IsPrint(rune(b)) {
			ch = string(rune(b))
		} else {
			ch = "."
		}

		if byteOffset == m.cursor {
			sb.WriteString(styleCursorByte.Render(ch))
		} else {
			sb.WriteString(ch)
		}
	}

	// Pad to full row width.
	for i := len(rowBytes); i < bytesPerRow; i++ {
		sb.WriteByte(' ')
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (m *Model) totalRows() int {
	if len(m.data) == 0 {
		return 1
	}
	return (len(m.data) + bytesPerRow - 1) / bytesPerRow
}

func (m *Model) visibleRows() int {
	// Approximate: header + footer + status = 3 lines.
	v := m.height - 3
	if v < 1 {
		v = 1
	}
	return v
}

// clampScroll ensures topRow keeps the cursor row visible.
func (m *Model) clampScroll() {
	cursorRow := m.cursor / bytesPerRow
	visible := m.visibleRows()
	totalRows := m.totalRows()

	if cursorRow < m.topRow {
		m.topRow = cursorRow
	}
	if cursorRow >= m.topRow+visible {
		m.topRow = cursorRow - visible + 1
	}
	if m.topRow < 0 {
		m.topRow = 0
	}
	maxTop := totalRows - visible
	if maxTop < 0 {
		maxTop = 0
	}
	if m.topRow > maxTop {
		m.topRow = maxTop
	}
}
