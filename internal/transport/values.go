package transport

import "errors"

// ErrInvalidAutomationMode is returned when parsing an unrecognized mode string.
var ErrInvalidAutomationMode = errors.New("transport: invalid automation mode")

// ParseAutomationMode converts a string to an AutomationMode.
func ParseAutomationMode(s string) (AutomationMode, error) {
	switch s {
	case "open":
		return ModeOpen, nil
	case "same-user":
		return ModeSameUser, nil
	case "children":
		return ModeChildren, nil
	default:
		return 0, ErrInvalidAutomationMode
	}
}
