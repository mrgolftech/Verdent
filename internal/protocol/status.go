package protocol

import (
	"regexp"
	"strings"
)

var accountSuspensionCode = regexp.MustCompile(`(?i)"error_code"\s*:\s*80006\b`)

func IsAccountSuspended(raw string) bool {
	if raw == "" { return false }
	lower := strings.ToLower(raw)
	return accountSuspensionCode.MatchString(raw) ||
		strings.Contains(lower, "free mode access has been suspended") ||
		strings.Contains(lower, "violation of our policies")
}

func IsRateLimit(status int, raw string) bool {
	if status == 429 { return true }
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "rate_limit") || strings.Contains(lower, "rate limit") || strings.Contains(lower, "rate-limit") { return true }
	if strings.Contains(lower, "quota") || strings.Contains(lower, "too many requests") || strings.Contains(lower, "resource exhausted") { return true }
	if strings.Contains(lower, "model limit reached") || strings.Contains(lower, "weekly limit") || strings.Contains(lower, "5-hour") || strings.Contains(lower, "5 hour") { return true }
	return strings.Contains(lower, "limit") && (strings.Contains(lower, "reached") || strings.Contains(lower, "weekly") || strings.Contains(lower, "free limit"))
}
