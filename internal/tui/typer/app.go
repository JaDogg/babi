// Package typer implements a touch-typing tutor TUI for babi.
//
// Inspired by gotypist by Paul Baecher and the teaching method described
// in Steve Yegge's blog post:
//   https://github.com/pb-/gotypist
//   http://steve-yegge.blogspot.com/2008/09/programmings-dirtiest-little-secret.html
package typer

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

//go:embed dictionary
var dictData []byte

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	failPenaltySeconds         = 3
	failPenaltyDuration        = time.Second * failPenaltySeconds
	fastErrorHighlightDuration = time.Millisecond * 333
	scoreHighlightDuration     = time.Second * 3
	tickInterval               = time.Millisecond * 100
)

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

var (
	colorCorrect = lipgloss.AdaptiveColor{Light: "#2E8B57", Dark: "#50FA7B"}
	colorWrong   = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}
	colorWrongBg = lipgloss.AdaptiveColor{Light: "#CC3333", Dark: "#FF5555"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#767676", Dark: "#626262"}
	colorPrimary = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	colorFaint   = lipgloss.AdaptiveColor{Light: "#DEDEDE", Dark: "#333333"}
	colorFast    = lipgloss.AdaptiveColor{Light: "#2E8B57", Dark: "#50FA7B"}
	colorSlow    = lipgloss.AdaptiveColor{Light: "#8B008B", Dark: "#FF79C6"}
	colorNormal  = lipgloss.AdaptiveColor{Light: "#B8860B", Dark: "#F1FA8C"}
	colorBlue    = lipgloss.AdaptiveColor{Light: "#0055BB", Dark: "#8BE9FD"}
	colorFail    = lipgloss.AdaptiveColor{Light: "#CC3333", Dark: "#FF5555"}

	styleHeaderBg    = lipgloss.NewStyle().Background(colorPrimary)
	styleHeaderTitle = lipgloss.NewStyle().Background(colorPrimary).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 2)
	styleHeaderRight = lipgloss.NewStyle().Background(colorPrimary).Foreground(lipgloss.Color("#CCC8FF")).Padding(0, 2)
	styleFooterBar   = lipgloss.NewStyle().Background(colorFaint).Foreground(colorMuted).Padding(0, 2)
	styleFooterKey   = lipgloss.NewStyle().Background(colorMuted).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1)
	styleSubtle      = lipgloss.NewStyle().Foreground(colorMuted)

	styleCorrectChar = lipgloss.NewStyle().Foreground(colorCorrect)
	styleWrongChar   = lipgloss.NewStyle().Foreground(colorWrong).Background(colorWrongBg)
	stylePlainChar   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#333333", Dark: "#CCCCCC"})
	stylePlainDim    = lipgloss.NewStyle().Foreground(colorMuted)
	styleFail        = lipgloss.NewStyle().Foreground(colorFail).Bold(true)
	styleScore       = lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	styleInfo        = lipgloss.NewStyle().Foreground(colorMuted)
)

// ---------------------------------------------------------------------------
// Core types
// ---------------------------------------------------------------------------

// Typo records a single keystroke error.
type Typo struct {
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// Round tracks a single pass through a phrase in one mode.
type Round struct {
	StartedAt  time.Time
	FailedAt   time.Time
	FinishedAt time.Time
	Errors     int
	Typos      []Typo
}

// Phrase is the current text being typed and the state of each round.
type Phrase struct {
	Text   string
	Input  string
	Rounds [3]Round
	Mode   Mode
}

func (p *Phrase) CurrentRound() *Round { return &p.Rounds[p.Mode] }

func (p *Phrase) ShowFail(now time.Time) bool {
	return p.Mode == ModeSlow && now.Sub(p.CurrentRound().FailedAt) < failPenaltyDuration
}

func (p *Phrase) expected() rune {
	if len(p.Input) >= len(p.Text) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(p.Text[len(p.Input):])
	return r
}

// ---------------------------------------------------------------------------
// tea messages
// ---------------------------------------------------------------------------

type tickMsg time.Time
type statsWrittenMsg struct{}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// Model is the bubbletea model for the typing tutor TUI.
type Model struct {
	phrase    Phrase
	score     float64
	lastScore float64
	lastPct   float64
	lastUntil time.Time
	repeat    bool
	seed      int64
	generator PhraseFunc
	statsFile string
	width     int
	height    int
	quitting  bool
}

// New creates a new Model, loading the dictionary and initial score.
// If phrase is non-empty it is used as a static phrase; if words are provided
// they are joined as the phrase. Otherwise the built-in dictionary is used.
func New(staticPhrase string, file string, codeMode bool, numProb float64) (Model, error) {
	var gen PhraseFunc

	switch {
	case staticPhrase != "":
		gen = StaticPhrase(staticPhrase)
	case file != "":
		data, err := os.ReadFile(file)
		if err != nil {
			return Model{}, fmt.Errorf("reading word file: %w", err)
		}
		gen = buildGenerator(data, codeMode, numProb)
	case codeMode:
		gen = buildGenerator(nil, true, numProb)
	default:
		gen = buildGenerator(dictData, false, numProb)
	}

	statsFile := DefaultStatsFile()
	score := LoadScore(statsFile)
	seed := time.Now().UnixNano()

	m := Model{
		generator: gen,
		statsFile: statsFile,
		score:     score,
		seed:      seed,
	}
	m = m.nextPhrase(false)
	return m, nil
}

func buildGenerator(data []byte, codeMode bool, numProb float64) PhraseFunc {
	if codeMode {
		// nil data means no file was provided — use the built-in multi-language pool
		if data == nil {
			return RandomLine(allCodeLines)
		}
		words := filterWords(readLines(data), `^[^/][^/]`, 80)
		if len(words) == 0 {
			return DefaultPhrase
		}
		return SequentialLine(words)
	}
	words := filterWords(readLines(data), `^[a-z]+$`, 8)
	if len(words) == 0 {
		return DefaultPhrase
	}
	return RandomPhrase(words, 30, numProb)
}

func (m Model) nextPhrase(forceNext bool) Model {
	if !m.repeat || forceNext {
		next, _ := m.generator(m.seed)
		m.seed = next
	}
	_, text := m.generator(m.seed)
	m.phrase = Phrase{Text: text}
	return m
}

// ---------------------------------------------------------------------------
// Init / Update / View
// ---------------------------------------------------------------------------

func (m Model) Init() tea.Cmd {
	return tick()
}

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func appendStatsCmd(path string, data []byte) tea.Cmd {
	return func() tea.Msg {
		_ = AppendStats(path, data)
		return statsWrittenMsg{}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		if m.quitting {
			return m, nil
		}
		return m, tick()

	case statsWrittenMsg:
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	now := time.Now()

	// Quit
	if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc {
		m.quitting = true
		return m, tea.Quit
	}

	// During fail penalty: ignore all input
	if m.phrase.ShowFail(now) {
		return m, nil
	}

	// Start round timer on first keypress
	if m.phrase.CurrentRound().StartedAt.IsZero() {
		m.phrase.CurrentRound().StartedAt = now
	}

	switch msg.Type {
	case tea.KeyBackspace:
		return m.handleBackspace(), nil

	case tea.KeyCtrlF:
		m = m.nextPhrase(true)
		return m, nil

	case tea.KeyCtrlR:
		m.repeat = !m.repeat
		return m, nil

	case tea.KeyEnter:
		return m.handleEnter(now)

	default:
		return m.handleChar(msg, now)
	}
}

func (m Model) handleBackspace() Model {
	if len(m.phrase.Input) == 0 {
		return m
	}
	_, l := utf8.DecodeLastRuneInString(m.phrase.Input)
	m.phrase.Input = m.phrase.Input[:len(m.phrase.Input)-l]
	return m
}

func (m Model) handleEnter(now time.Time) (tea.Model, tea.Cmd) {
	if m.phrase.Input != m.phrase.Text {
		return m, nil
	}

	statsData := formatStats(&m.phrase, now)
	writeCmd := appendStatsCmd(m.statsFile, statsData)
	m.phrase.CurrentRound().FinishedAt = now

	if m.phrase.Mode != ModeNormal {
		m.phrase.Mode++
		m.phrase.Input = ""
		return m, writeCmd
	}

	// All three rounds done — compute score
	m.lastUntil = now.Add(scoreHighlightDuration)
	gained := computePhraseScore(m.phrase)
	m.lastScore = gained
	m.lastPct = gained / maxScore(m.phrase.Text)
	m.score += gained
	m = m.nextPhrase(false)

	return m, writeCmd
}

func (m Model) handleChar(msg tea.KeyMsg, now time.Time) (tea.Model, tea.Cmd) {
	var ch rune
	if msg.Type == tea.KeySpace {
		ch = ' '
	} else if len(msg.Runes) > 0 {
		ch = msg.Runes[0]
	}
	if ch == 0 {
		return m, nil
	}

	exp := m.phrase.expected()

	if ch == exp {
		m.phrase.Input += string(ch)
		return m, nil
	}

	// Typo
	if exp != 0 {
		m.phrase.CurrentRound().Typos = append(m.phrase.CurrentRound().Typos, Typo{
			Expected: string(exp),
			Actual:   string(ch),
		})
	}
	m.phrase.CurrentRound().Errors++
	m.phrase.CurrentRound().FailedAt = now

	if m.phrase.Mode == ModeFast {
		// Accept wrong chars in fast mode, just highlight briefly
		m.phrase.Input += string(ch)
		return m, nil
	}

	if m.phrase.Mode == ModeSlow {
		// Reset input with penalty
		m.phrase.Input = ""
		return m, nil
	}

	// Normal mode: accept wrong chars
	m.phrase.Input += string(ch)
	return m, nil
}

func computePhraseScore(phrase Phrase) float64 {
	var scores [3]float64
	for mode, round := range phrase.Rounds {
		d := round.FinishedAt.Sub(round.StartedAt)
		switch Mode(mode) {
		case ModeFast:
			scores[mode] = speedScore(phrase.Text, d)
		case ModeSlow:
			scores[mode] = errorScore(round.Errors)
		case ModeNormal:
			scores[mode] = combinedScore(phrase.Text, d, round.Errors)
		}
	}
	return finalScore(phrase.Text, scores[0], scores[1], scores[2])
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m Model) View() string {
	if m.width == 0 {
		return "Loading…"
	}
	if m.quitting {
		return ""
	}

	now := time.Now()

	header := m.renderHeader()
	footer := m.renderFooter()

	body := strings.Join([]string{
		m.renderModeInfo(),
		"",
		m.renderPhrase(now),
		"",
		m.renderStats(now),
		"",
		m.renderScoreBar(now),
		m.renderProgress(),
		"",
		m.renderAttribution(),
	}, "\n")

	bodyH := strings.Count(body, "\n") + 1
	// available rows between header and footer
	available := m.height - 2
	if available < bodyH {
		available = bodyH
	}
	topPad := (available - bodyH) / 2
	botPad := available - bodyH - topPad

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString(strings.Repeat("\n", topPad+1))
	sb.WriteString(body)
	sb.WriteString(strings.Repeat("\n", botPad+1))
	sb.WriteString(footer)
	return sb.String()
}

func (m Model) renderHeader() string {
	title := styleHeaderTitle.Render("babi type")
	mode := styleHeaderRight.Render(m.phrase.Mode.Name() + " mode")

	titleW := lipgloss.Width(title)
	modeW := lipgloss.Width(mode)
	gap := m.width - titleW - modeW
	if gap < 0 {
		gap = 0
	}
	fill := styleHeaderBg.Render(strings.Repeat(" ", gap))
	return title + fill + mode
}

func (m Model) renderModeInfo() string {
	modeStyle := lipgloss.NewStyle().Bold(true)
	switch m.phrase.Mode {
	case ModeFast:
		modeStyle = modeStyle.Foreground(colorFast)
	case ModeSlow:
		modeStyle = modeStyle.Foreground(colorSlow)
	case ModeNormal:
		modeStyle = modeStyle.Foreground(colorNormal)
	}

	modeName := modeStyle.Render(strings.ToUpper(m.phrase.Mode.Name()))
	desc := styleInfo.Render("  " + m.phrase.Mode.Desc())
	line := modeName + desc
	return center(line, m.width)
}

func (m Model) renderPhrase(now time.Time) string {
	if m.phrase.ShowFail(now) {
		left := int(m.phrase.CurrentRound().FailedAt.Add(failPenaltyDuration).Sub(now).Seconds()) + 1
		if left < 1 {
			left = 1
		}
		msg := failMessage(m.phrase.CurrentRound().Errors)
		line := styleFail.Render(fmt.Sprintf(msg, left))
		return center(line, m.width)
	}

	_, runeOff := errorOffset(m.phrase.Text, m.phrase.Input)
	textRunes := []rune(m.phrase.Text)
	inputRunes := []rune(m.phrase.Input)

	var sb strings.Builder
	for i, ch := range textRunes {
		char := string(ch)
		if i < runeOff {
			sb.WriteString(styleCorrectChar.Render(char))
		} else if i < len(inputRunes) {
			sb.WriteString(styleWrongChar.Render(char))
		} else {
			sb.WriteString(stylePlainChar.Render(char))
		}
	}
	sb.WriteString(stylePlainDim.Render("⏎"))

	return center(sb.String(), m.width)
}

func (m Model) renderStats(now time.Time) string {
	byteOff, _ := errorOffset(m.phrase.Text, m.phrase.Input)
	correctInput := m.phrase.Input
	if byteOff < len(m.phrase.Input) {
		correctInput = m.phrase.Input[:byteOff]
	}
	seconds, _, wpm := computeStats(correctInput, m.phrase.CurrentRound().StartedAt, now)
	errors := m.phrase.CurrentRound().Errors

	errStyle := styleInfo
	if m.phrase.Mode == ModeFast &&
		now.Sub(m.phrase.CurrentRound().FailedAt) < fastErrorHighlightDuration {
		errStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "#B8860B", Dark: "#F1FA8C"}).Bold(true)
	}

	errStr := errStyle.Render(fmt.Sprintf("%d errors", errors))
	timeStr := styleInfo.Render(fmt.Sprintf("%.1fs", seconds))
	wpmStr := styleInfo.Render(fmt.Sprintf("%.0f wpm", wpm))

	sep := styleInfo.Render("  ·  ")
	line := errStr + sep + timeStr + sep + wpmStr
	return center(line, m.width)
}

func (m Model) renderScoreBar(now time.Time) string {
	var parts []string

	if now.Before(m.lastUntil) {
		gained := fmt.Sprintf("+%.0f (%.0f%%)", m.lastScore, 100*m.lastPct)
		parts = append(parts, styleScore.Render(
			fmt.Sprintf("Score: %.0f  %s", m.score, gained)))
	} else {
		parts = append(parts, styleScore.Render(fmt.Sprintf("Score: %.0f", m.score)))
	}

	lvl := scoreLevel(m.score)
	if now.Before(m.lastUntil) && scoreLevel(m.score-m.lastScore) != lvl {
		parts = append(parts, styleScore.Render(fmt.Sprintf("  Level %d  ↑ level up!", lvl)))
	} else {
		parts = append(parts, styleInfo.Render(fmt.Sprintf("  Level %d", lvl)))
	}

	if m.repeat {
		parts = append(parts, styleInfo.Render("  [repeating]"))
	}

	return center(strings.Join(parts, ""), m.width)
}

func (m Model) renderProgress() string {
	pct := scoreProgress(m.score)
	barWidth := 30
	filled := int(float64(barWidth) * pct)
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	lvl := scoreLevel(m.score)
	line := styleInfo.Render(fmt.Sprintf("Level %d  [%s]  %.0f%%", lvl, bar, 100*pct))
	return center(line, m.width)
}

func (m Model) renderAttribution() string {
	line := styleSubtle.Render("Based on gotypist by Paul Baecher · http://steve-yegge.blogspot.com/2008/09/programmings-dirtiest-little-secret.html")
	return center(line, m.width)
}

func (m Model) renderFooter() string {
	type kv struct{ key, desc string }
	pairs := []kv{
		{"enter", "submit"},
		{"backspace", "delete"},
		{"ctrl+f", "skip"},
		{"ctrl+r", "repeat"},
		{"esc", "quit"},
	}

	var parts []string
	for _, p := range pairs {
		k := styleFooterKey.Render(p.key)
		d := styleSubtle.Render(" " + p.desc + "  ")
		parts = append(parts, k+d)
	}
	content := strings.Join(parts, "")
	pad := m.width - lipgloss.Width(content)
	if pad > 0 {
		content += strings.Repeat(" ", pad)
	}
	return styleFooterBar.Render(content)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func center(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	pad := (width - w) / 2
	return strings.Repeat(" ", pad) + s
}

// errorOffset returns the byte and rune offset of the first error in input
// compared to text. If there are no errors it returns the end of input.
func errorOffset(text, input string) (byteOffset, runeOffset int) {
	runeOff := 0
	for i, tr := range text {
		if i >= len(input) {
			return len(input), runeOff
		}
		ir, _ := utf8.DecodeRuneInString(input[i:])
		if ir != tr {
			return i, runeOff
		}
		runeOff++
	}
	l := len(input)
	if l > len(text) {
		l = len(text)
	}
	return l, runeOff
}

func failMessage(errs int) string {
	switch errs {
	case 1:
		return "Not quite! Try again in %d..."
	case 2, 3:
		return "FAIL! Let's do this again in %d..."
	case 4, 5:
		return "Dude?! Try again in %d..."
	case 6, 7, 8:
		return "Are you serious?!? Again in %d..."
	default:
		return "I don't even... %d..."
	}
}

// ---------------------------------------------------------------------------
// Command
// ---------------------------------------------------------------------------

// Command returns the cobra command for babi type.
func Command() *cobra.Command {
	var file string
	var codeMode bool
	var numProb float64

	cmd := &cobra.Command{
		Use:   "type [word...]",
		Short: "Touch-typing tutor TUI",
		Long: `Touch-typing tutor — practice the fast/slow/normal method.

Based on gotypist (https://github.com/pb-/gotypist) by Paul Baecher,
implementing the teaching methodology from Steve Yegge's blog post:
http://steve-yegge.blogspot.com/2008/09/programmings-dirtiest-little-secret.html

Each phrase is typed in three modes:
  fast   — type as fast as you can, errors are accepted
  slow   — go slow, any mistake resets with a penalty
  normal — balance speed and accuracy`,
		RunE: func(cmd *cobra.Command, args []string) error {
			static := strings.Join(args, " ")
			m, err := New(static, file, codeMode, numProb)
			if err != nil {
				return err
			}
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "word list file (one word per line); use - for stdin")
	cmd.Flags().BoolVarP(&codeMode, "code", "c", false, "treat file as code lines (sequential mode)")
	cmd.Flags().Float64VarP(&numProb, "numbers", "n", 0, "probability of inserting random numbers (0-1)")

	return cmd
}
