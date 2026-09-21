package protocol

import "testing"

func TestStatusClassification(t *testing.T) {
	if !IsAccountSuspended(`{"error_code":80006,"msg":"Due to a violation of our policies"}`) { t.Fatal("expected suspension") }
	if IsAccountSuspended(`{"error_code":429}`) { t.Fatal("429 must not be suspension") }
	if !IsRateLimit(200, `{"error":"weekly limit reached"}`) { t.Fatal("expected in-band rate limit") }
	if !IsRateLimit(429, "") { t.Fatal("HTTP 429 must be rate limit") }
	if IsRateLimit(400, "context limit exceeded") { t.Fatal("context limit must not be treated as quota rate limit") }
}
