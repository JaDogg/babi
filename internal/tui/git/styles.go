package git

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#02A699", Dark: "#04D18A"}
	colorError   = lipgloss.AdaptiveColor{Light: "#CC3333", Dark: "#FF5555"}
	colorWarning = lipgloss.AdaptiveColor{Light: "#D4A800", Dark: "#FFCC00"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#767676", Dark: "#626262"}
	colorFaint   = lipgloss.AdaptiveColor{Light: "#DEDEDE", Dark: "#333333"}
	colorBg      = lipgloss.AdaptiveColor{Light: "#F0EEFF", Dark: "#1E1A2E"}

	styleHeaderBg = lipgloss.NewStyle().
			Background(colorPrimary)

	styleHeaderTitle = lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				Padding(0, 2)

	styleHeaderRight = lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(lipgloss.Color("#CCC8FF")).
				Padding(0, 2)

	styleFooterBar = lipgloss.NewStyle().
			Background(colorFaint).
			Foreground(colorMuted).
			Padding(0, 2)

	styleFooterKey = lipgloss.NewStyle().
			Background(colorMuted).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1)

	styleSuccess  = lipgloss.NewStyle().Foreground(colorSuccess)
	styleError    = lipgloss.NewStyle().Foreground(colorError)
	styleWarning  = lipgloss.NewStyle().Foreground(colorWarning)
	styleSubtle   = lipgloss.NewStyle().Foreground(colorMuted)
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	styleLabel    = lipgloss.NewStyle().Bold(true)

	styleDirHeader = lipgloss.NewStyle().
			Foreground(colorMuted).
			Bold(true)

	styleCheckboxOn  = lipgloss.NewStyle().Foreground(colorSuccess)
	styleCheckboxOff = lipgloss.NewStyle().Foreground(colorMuted)
	styleCheckboxMix = lipgloss.NewStyle().Foreground(colorWarning)

	styleActiveInput = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Padding(0, 1)

	styleInactiveInput = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorFaint).
				Padding(0, 1)

	styleTypeSelected = lipgloss.NewStyle().
				Background(colorBg).
				Foreground(colorPrimary).
				Bold(true)

	styleTypeCursor = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	stylePreviewBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFaint).
			Padding(0, 2)

	styleDiffAdd = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1a7f37", Dark: "#3fb950"})
	styleDiffDel = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"})
)
