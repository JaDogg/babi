package git

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func padToHeight(s string, height int) string {
	s = strings.TrimRight(s, "\n")
	n := strings.Count(s, "\n") + 1
	if n < height {
		s += strings.Repeat("\n", height-n)
	}
	return s
}

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

func renderFooter(width int, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		k := styleFooterKey.Render(pairs[i])
		parts = append(parts, k+"  "+pairs[i+1])
	}
	content := strings.Join(parts, "   ")
	return styleFooterBar.Width(width - 4).Render(content)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
