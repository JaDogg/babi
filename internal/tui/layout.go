package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// padToHeight ensures s contains exactly height visual lines (no trailing newline).
// Used to guarantee content areas fill the terminal height precisely.
func padToHeight(s string, height int) string {
	s = strings.TrimRight(s, "\n")
	n := strings.Count(s, "\n") + 1
	if n < height {
		s += strings.Repeat("\n", height-n)
	}
	return s
}

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
//
// styleFooterBar has Padding(0,2) = 4 horizontal chars overhead.
// We set Width(width-4) so the total rendered width == width.
func renderFooter(width int, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		k := styleFooterKey.Render(pairs[i])
		parts = append(parts, k+"  "+pairs[i+1])
	}
	content := strings.Join(parts, "   ")
	return styleFooterBar.Width(width - 4).Render(content)
}

// renderSplit lays out sidebar | main at exact terminal width.
//
// styleSidebar overhead (horizontal): left-pad 3 + right-pad 2 + border-right 1 = 6
// styleSidebar overhead (vertical):   top-pad 1 + bottom-pad 1 = 2
// mainPane Padding(0,1): left 1 + right 1 = 2 horizontal, 0 vertical
//
// With sideTotal = 28:
//   sidebar content width  = 28 - 6 = 22
//   main content width     = totalWidth - 28 - 2 = totalWidth - 30
//   total                  = (22+6) + (totalWidth-30+2) = totalWidth ✓
//   sidebar total height   = (contentH-2) + 2 = contentH ✓
//   main total height      = contentH ✓
const sideTotal = 28

func renderSplit(totalWidth, contentH int, sidebar, main string) string {
	const sideHOverhead = 6 // padding + border
	const sideVOverhead = 2 // top + bottom padding

	sideContentW := sideTotal - sideHOverhead // 22
	mainContentW := totalWidth - sideTotal - 2 // -2 for Padding(0,1)
	if mainContentW < 10 {
		mainContentW = 10
	}
	sideContentH := contentH - sideVOverhead
	if sideContentH < 1 {
		sideContentH = 1
	}

	sidePane := styleSidebar.
		Width(sideContentW).
		Height(sideContentH).
		Render(sidebar)

	mainPane := lipgloss.NewStyle().
		Width(mainContentW).
		Height(contentH).
		Padding(0, 1).
		Render(main)

	return lipgloss.JoinHorizontal(lipgloss.Top, sidePane, mainPane)
}
