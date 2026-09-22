package account

import (
	"time"

	"github.com/mrgolftech/Verdent/internal/protocol"
	"github.com/mrgolftech/Verdent/internal/transport"
)

// NewProtocolClient builds a Verdent client whose outbound transport is owned
// by exactly one account. This is the enforcement point for proxy affinity.
func NewProtocolClient(a Account, base protocol.Config, timeout time.Duration) (*protocol.Client, error) {
	httpClient, err := transport.NewHTTPClient(a.Credential.ProxyURL, timeout)
	if err != nil { return nil, err }
	cfg := base
	cfg.DeviceID = a.Credential.DeviceID
	if a.Credential.TeamID != "" { cfg.TeamID = a.Credential.TeamID }
	return protocol.NewClient(cfg,httpClient)
}
