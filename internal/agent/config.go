package agent

import "github.com/anthropics/anthropic-sdk-go"

// DefaultModel is used unless -model overrides it, e.g. -model claude-opus-5-5.
const DefaultModel = anthropic.ModelClaudeSonnet5_5
