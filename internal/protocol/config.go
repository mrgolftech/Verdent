package protocol

import (
	"errors"
	"runtime"
	"strings"

	"github.com/mrgolftech/Verdent/internal/canonical"
)

const (
	DefaultEndpoint = "https://llm-proxy.verdent.ai/llm/stream"
	DefaultCatalogEndpoint = "https://llm-proxy.verdent.ai/config/model_list"
)

type Config struct {
	Endpoint      string
	CatalogEndpoint string
	AppVersion    string
	BetaHeader    string
	Sign          string
	Channel       string
	AgentName     string
	ReactType     string
	DeviceID      string
	DeviceModel   string
	DeviceType    string
	OSType        string
	OSName        string
	CPUArch       string
	TeamID        string
	UserAgent     string
	NativeAPI     bool
	SystemTrailer []canonical.ContentBlock
}

func (c Config) WithDefaults() Config {
	if strings.TrimSpace(c.Endpoint) == "" { c.Endpoint = DefaultEndpoint }
	if strings.TrimSpace(c.CatalogEndpoint) == "" { c.CatalogEndpoint = DefaultCatalogEndpoint }
	if c.Channel == "" { c.Channel = "deck" }
	if c.AgentName == "" { c.AgentName = "VerdentDeck" }
	if c.ReactType == "" { c.ReactType = "Main Agent" }
	if c.DeviceType == "" { c.DeviceType = "pc" }
	if c.OSType == "" {
		switch runtime.GOOS {
		case "windows": c.OSType = "windows"
		case "darwin": c.OSType = "macos"
		default: c.OSType = "linux"
		}
	}
	if c.CPUArch == "" { c.CPUArch = runtime.GOARCH }
	if c.TeamID == "" { c.TeamID = "0" }
	return c
}

func (c Config) Validate() error {
	c = c.WithDefaults()
	if c.Sign == "" { return errors.New("protocol sign is required") }
	if c.AppVersion == "" { return errors.New("Verdent app version is required") }
	if c.BetaHeader == "" { return errors.New("Verdent beta protocol header is required") }
	if c.DeviceID == "" { return errors.New("device ID is required") }
	return nil
}
