package server

import (
	"time"

	"github.com/mrgolftech/Verdent/internal/protocol"
)

// recordUpstreamFailure classifies Verdent failures and updates account routing
// state. Account suspension is terminal until an operator explicitly re-enables
// the account; transient rate limits enter cooldown.
func (s *Server) recordUpstreamFailure(accountID string, status int, raw, retryAfterHeader string) string {
	if protocol.IsAccountSuspended(raw) {
		s.Accounts.MarkSuspended(accountID, raw)
		return "verdent_account_suspended"
	}
	if protocol.IsRateLimit(status, raw) {
		s.Accounts.MarkRateLimited(accountID, time.Now().Add(retryAfter(retryAfterHeader)), raw)
		return "verdent_rate_limit"
	}
	return "upstream_error"
}
