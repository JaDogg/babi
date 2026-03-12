package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#02A699", Dark: "#04D18A"}
	colorError   = lipgloss.AdaptiveColor{Light: "#CC3333", Dark: "#FF5555"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#767676", Dark: "#626262"}
	colorFaint   = lipgloss.AdaptiveColor{Light: "#DEDEDE", Dark: "#333333"}
	colorBg      = lipgloss.AdaptiveColor{Light: "#F0EEFF", Dark: "#1E1A2E"}

	// Header bar
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

	// Footer / help bar
	styleFooterBar = lipgloss.NewStyle().
			Background(colorFaint).
			Foreground(colorMuted).
			Padding(0, 2)

	styleFooterKey = lipgloss.NewStyle().
			Background(colorMuted).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1)

	// Content area
	styleContent = lipgloss.NewStyle().Padding(1, 3)

	// List / table rows
	styleColHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorMuted).
			PaddingBottom(1)

	styleRowNormal = lipgloss.NewStyle()

	styleRowSelected = lipgloss.NewStyle().
				Background(colorBg).
				Foreground(colorPrimary).
				Bold(true)

	styleRowCursor = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	// Status indicators
	styleSuccess = lipgloss.NewStyle().Foreground(colorSuccess)
	styleError   = lipgloss.NewStyle().Foreground(colorError)
	styleSubtle  = lipgloss.NewStyle().Foreground(colorMuted)
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)

	// Add screen sidebar
	styleSidebar = lipgloss.NewStyle().
			BorderRight(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorFaint).
			Padding(1, 2, 1, 3)

	styleStepActive  = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	styleStepDone    = lipgloss.NewStyle().Foreground(colorSuccess)
	styleStepPending = lipgloss.NewStyle().Foreground(colorMuted)

	// Form elements
	styleLabel      = lipgloss.NewStyle().Bold(true)
	styleTitle      = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary) // compat
	styleHelp       = lipgloss.NewStyle().Foreground(colorMuted)              // compat

	styleActiveInput = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Padding(0, 1)

	styleInactiveInput = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorFaint).
				Padding(0, 1)

	styleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFaint).
			Padding(1, 3)
)
