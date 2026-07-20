package daemon

import "strings"

func classifyAgentFailure(provider, errMsg string) string {
	s := strings.ToLower(strings.TrimSpace(errMsg))
	if s == "" {
		return "agent_error"
	}

	if strings.Contains(s, "access token could not be refreshed") ||
		strings.Contains(s, "token_expired") ||
		strings.Contains(s, "provided authentication token is expired") ||
		strings.Contains(s, "please log out and sign in again") ||
		(strings.Contains(s, "401 unauthorized") && (strings.Contains(s, "auth") || strings.Contains(s, "token"))) {
		return "auth_expired"
	}

	if strings.Contains(s, "rate limit") ||
		strings.Contains(s, "too many requests") ||
		strings.Contains(s, "429") {
		return "rate_limit"
	}

	if strings.Contains(s, "insufficient_quota") ||
		strings.Contains(s, "quota exceeded") ||
		strings.Contains(s, "billing") ||
		strings.Contains(s, "usage limit") {
		return "quota_exceeded"
	}

	if strings.Contains(s, "context length") ||
		strings.Contains(s, "maximum context") ||
		strings.Contains(s, "context window") ||
		strings.Contains(s, "too many tokens") {
		return "context_limit"
	}

	if strings.Contains(s, "model") {
		if strings.Contains(s, "access denied") ||
			strings.Contains(s, "not available for this account") ||
			strings.Contains(s, "not available to your account") ||
			strings.Contains(s, "do not have access") ||
			strings.Contains(s, "don't have access") {
			return "model_access"
		}

		if strings.Contains(s, "not found") ||
			strings.Contains(s, "does not exist") {
			return "model_not_found"
		}

		if strings.Contains(s, "not available") ||
			strings.Contains(s, "unsupported") ||
			strings.Contains(s, "not enabled") {
			return "model_limit"
		}
	}

	_ = provider // Reserved for provider-specific refinements.
	return "agent_error"
}
