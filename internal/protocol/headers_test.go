package protocol

import "testing"

func TestBuildHeaders(t *testing.T) {
	cfg := Config{
		AppVersion:"2.test", BetaHeader:"beta-test", Sign:"protocol-sign-for-test-only", DeviceID:"device-1",
		DeviceModel:"cpu-x", OSName:"Linux test", CPUArch:"amd64", TeamID:"team-1", UserAgent:"Verdent/test",
	}
	h, err := BuildHeaders("secret-token", cfg)
	if err != nil { t.Fatal(err) }
	checks := map[string]string{
		"Authorization":"Bearer secret-token", "Cookie":"token=secret-token", "verdent-proxy-beta":"beta-test",
		"X-Version-Code":"2.test", "X-Device-ID":"device-1", "X-Team-ID":"team-1", "Device-Model":"cpu-x",
		"OS":"Linux test", "CPU-Arch":"amd64", "User-Agent":"Verdent/test", "agent_type":"ts_agent",
	}
	for k, want := range checks {
		if got := h.Get(k); got != want { t.Fatalf("%s: want %q got %q", k, want, got) }
	}
}
