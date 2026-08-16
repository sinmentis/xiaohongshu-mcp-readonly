package xiaohongshu

import (
	"fmt"
	"strings"
)

// InvalidArgumentError reports a caller-correctable public input error.
type InvalidArgumentError struct {
	Field     string
	Value     string
	Supported []string
}

func (e *InvalidArgumentError) Error() string {
	if len(e.Supported) == 0 {
		return fmt.Sprintf("invalid %s %q", e.Field, e.Value)
	}
	return fmt.Sprintf(
		"invalid %s %q; supported values are %s",
		e.Field,
		e.Value,
		strings.Join(e.Supported, ", "),
	)
}
