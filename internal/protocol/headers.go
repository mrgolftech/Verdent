package protocol

import (
	"fmt"
	"net/http"
)

func BuildHeaders(token string, cfg Config) (http.Header, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil { return nil, err }
	if token == "" { return nil, fmt.Errorf("access token is required") }

	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "*/*")
	h.Set("Authorization", "Bearer "+token)
	h.Set("Cookie", "token="+token)
	h.Set("verdent-proxy-beta", cfg.BetaHeader)
	h.Set("X-Version-Code", cfg.AppVersion)
	h.Set("X-Device-ID", cfg.DeviceID)
	h.Set("X-Team-ID", cfg.TeamID)
	h.Set("X-Device-Type", cfg.DeviceType)
	h.Set("X-OS-Type", cfg.OSType)
	if cfg.OSName != "" { h.Set("OS", cfg.OSName) }
	if cfg.CPUArch != "" { h.Set("CPU-Arch", cfg.CPUArch) }
	if cfg.DeviceModel != "" { h.Set("Device-Model", cfg.DeviceModel) }
	if cfg.UserAgent != "" { h.Set("User-Agent", cfg.UserAgent) }
	h.Set("agent_type", "ts_agent")
	return h, nil
}
