package ui

// Repeated key strings and help tokens, centralized to avoid magic-string drift
// and to satisfy goconst without scattering string literals across the package.
const (
	keyDown = "down"

	helpAzLogin       = "a az login"
	helpLogs          = "g logs"
	helpBackspaceBack = "backspace back"
	helpFilter        = "f|/ filter"
)
