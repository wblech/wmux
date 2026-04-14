package client

// listConfig holds resolved options for a List call.
type listConfig struct {
	prefix string
}

// ListOption configures a List call.
type ListOption func(*listConfig)

// WithListPrefix filters the list to sessions matching the given prefix.
func WithListPrefix(prefix string) ListOption {
	return func(c *listConfig) {
		c.prefix = prefix
	}
}

// KillPrefixResult holds the result of a batch kill-by-prefix operation.
type KillPrefixResult struct {
	Killed []string          `json:"killed"`
	Errors map[string]string `json:"errors,omitempty"`
}
