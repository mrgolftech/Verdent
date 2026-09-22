package account

import (
	"sync"
	"time"

	"github.com/mrgolftech/Verdent/internal/protocol"
	"github.com/mrgolftech/Verdent/internal/transport"
)

var accountRequestGates sync.Map

// NewProtocolClient builds a Verdent client whose outbound transport is owned
// by exactly one account. This is the enforcement point for proxy affinity.
//
// The upstream request gate is also keyed by account id. This preserves the
// observed Desktop pacing behavior without serializing unrelated accounts.
func NewProtocolClient(a Account, base protocol.Config, timeout time.Duration) (*protocol.Client, error) {
	httpClient, err := transport.NewHTTPClient(a.Credential.ProxyURL, timeout)
	if err != nil { return nil, err }
	cfg := base
	cfg.DeviceID = a.Credential.DeviceID
	if a.Credential.TeamID != "" { cfg.TeamID = a.Credential.TeamID }

	key := a.Credential.ID
	if key == "" {
		key = a.Credential.DeviceID
	}
	value, _ := accountRequestGates.LoadOrStore(key, protocol.NewRequestGate(cfg.MinRequestInterval))
	gate, _ := value.(*protocol.RequestGate)
	return protocol.NewClientWithGate(cfg,httpClient,gate)
}
