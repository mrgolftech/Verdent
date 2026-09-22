package verdentauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func ReadDesktopToken(ctx context.Context) (string, error) {
	var out []byte
	var err error
	switch runtime.GOOS {
	case "darwin":
		out, err = exec.CommandContext(ctx, "security", "find-generic-password", "-s", "ai.verdent.deck", "-a", "access-token", "-w").Output()
	case "linux":
		out, err = exec.CommandContext(ctx, "secret-tool", "lookup", "service", "ai.verdent.deck", "account", "access-token").Output()
	case "windows":
		return "", errors.New("automatic Windows Credential Manager import is not enabled yet; use browser sign-in or manual token import")
	default:
		return "", fmt.Errorf("Verdent desktop import is not supported on %s", runtime.GOOS)
	}
	if err != nil {
		return "", fmt.Errorf("read Verdent desktop credential: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return "", errors.New("Verdent desktop credential is empty")
	}
	var wrapper struct {
		AccessToken string `json:"accessToken"`
	}
	if json.Unmarshal([]byte(raw), &wrapper) == nil && strings.TrimSpace(wrapper.AccessToken) != "" {
		return strings.TrimSpace(wrapper.AccessToken), nil
	}
	return raw, nil
}
