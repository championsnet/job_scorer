package config

import "strings"

// ReasoningEffortFor returns the `reasoning_effort` value to send for a given
// OpenAI model so that reasoning is turned off (or minimized) for our cheap
// scoring task. It returns "" for non-reasoning models, which do not accept the
// parameter at all.
//
//   - GPT-5 family (incl. gpt-5.4) accepts "none", which disables reasoning.
//   - o-series (o1/o3/o4) has no "none"; its minimum is "low".
func ReasoningEffortFor(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "gpt-5"):
		return "none"
	case strings.HasPrefix(m, "o1"), strings.HasPrefix(m, "o3"), strings.HasPrefix(m, "o4"):
		return "low"
	default:
		return ""
	}
}

// IsReasoningModel reports whether a model is an OpenAI reasoning model.
func IsReasoningModel(model string) bool {
	return ReasoningEffortFor(model) != ""
}
