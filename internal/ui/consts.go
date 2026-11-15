package ui

// Repeated key strings and help tokens, centralized to avoid magic-string drift
// and to satisfy goconst without scattering string literals across the package.
const (
	keyDown = "down"

	helpAzLogin       = "a az login"
	helpEnsure        = "l ensure"
	helpEnterOpen     = "enter open"
	helpLogs          = "g logs"
	helpShell         = "s shell"
	helpBackspaceBack = "backspace back"
	helpFilter        = "f|/ filter"
)
