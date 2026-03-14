// only pure code in this file (no side effects)
package typer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Mode represents a typing mode.
type Mode int

const (
	ModeFast   Mode = 0
	ModeSlow   Mode = 1
	ModeNormal Mode = 2
)

var modeInfo = []struct {
	Name string
	Desc string
}{
	{Name: "fast", Desc: "type as fast as you can, ignore mistakes"},
	{Name: "slow", Desc: "go slow, do not make any mistake"},
	{Name: "normal", Desc: "type at normal speed, avoid mistakes"},
}

func (m Mode) Name() string { return modeInfo[m].Name }
func (m Mode) Desc() string { return modeInfo[m].Desc }

// Statistics is one recorded typing session (one phrase, one mode).
type Statistics struct {
	Text       string    `json:"text"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Errors     int       `json:"errors"`
	Typos      []Typo    `json:"typos"`
	Mode       Mode      `json:"mode"`
	Seconds    float64   `json:"seconds"`
	CPS        float64   `json:"cps"`
	WPM        float64   `json:"wpm"`
	Version    int       `json:"version"`
}

func computeStats(text string, start, end time.Time) (seconds, cps, wpm float64) {
	if !start.IsZero() {
		seconds = end.Sub(start).Seconds()
		if seconds > 0. {
			runeCount := utf8.RuneCountInString(text)
			cps = float64(runeCount) / seconds
			wordCount := len(strings.Split(text, " "))
			wpm = float64(wordCount) * 60 / seconds
		}
	}
	return
}

func formatStats(phrase *Phrase, now time.Time) []byte {
	typos := phrase.CurrentRound().Typos
	if typos == nil {
		typos = make([]Typo, 0)
	}

	seconds, cps, wpm := computeStats(
		phrase.Text, phrase.CurrentRound().StartedAt, now)
	stats := Statistics{
		Text:       phrase.Text,
		StartedAt:  phrase.CurrentRound().StartedAt,
		FinishedAt: now,
		Errors:     phrase.CurrentRound().Errors,
		Typos:      typos,
		Mode:       phrase.Mode,
		Seconds:    seconds,
		CPS:        cps,
		WPM:        wpm,
		Version:    1,
	}

	data, err := json.Marshal(stats)
	if err != nil {
		panic(err)
	}
	return append(data, '\n')
}

func computeTotalScore(data []byte) float64 {
	reader := bufio.NewReader(bytes.NewBuffer(data))
	var stats []Statistics
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			return 0
		}
		var s Statistics
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &s) == nil {
			stats = append(stats, s)
		}
	}

	total := 0.0
	for i := 0; i+2 < len(stats); i++ {
		fast, slow, normal := stats[i], stats[i+1], stats[i+2]
		if fast.Mode != ModeFast || slow.Mode != ModeSlow || normal.Mode != ModeNormal {
			continue
		}
		if fast.Text != slow.Text || slow.Text != normal.Text {
			continue
		}
		total += finalScore(
			fast.Text,
			speedScore(fast.Text, fast.FinishedAt.Sub(fast.StartedAt)),
			errorScore(slow.Errors),
			combinedScore(normal.Text, normal.FinishedAt.Sub(normal.StartedAt), normal.Errors),
		)
	}
	return total
}

// DefaultStatsFile returns the default path for the stats file.
func DefaultStatsFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".babi", "typer.stats")
}

// LoadScore reads the stats file and returns the total accumulated score.
func LoadScore(path string) float64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return computeTotalScore(data)
}

// AppendStats appends a stats entry to the stats file.
func AppendStats(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
