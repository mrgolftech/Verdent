package webui

import (
	"strings"
	"testing"
)

func TestAdminAppUsesCollectionSelectorForForEachBindings(t *testing.T) {
	data, err := embedded.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)

	// $() is querySelector and returns one Element. Any "$(...).forEach(...)"
	// binding is therefore a runtime bug. Keep this generic so newly added
	// account/table actions cannot reintroduce the same regression elsewhere.
	for lineNo, line := range strings.Split(js, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "$(") && strings.Contains(trimmed, ").forEach(") {
			t.Fatalf("single-element selector used with forEach at app.js:%d: %s", lineNo+1, trimmed)
		}
	}

	required := []string{
		`$$(".page").forEach`,
		`$$("[data-page]").forEach`,
		`$$("[data-close]").forEach`,
		`$$("[data-copy]").forEach`,
	}
	for _, pattern := range required {
		if !strings.Contains(js, pattern) {
			t.Fatalf("expected collection selector binding missing: %s", pattern)
		}
	}
}


func TestAPIPageOwnsKeyManagement(t *testing.T) {
	indexData, err := embedded.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	index := string(indexData)

	for _, forbidden := range []string{
		`data-page="keys"`,
		`id="page-keys"`,
		`Authorization: Bearer &lt;VERDENT_API_KEY&gt;`,
	} {
		if strings.Contains(index, forbidden) {
			t.Fatalf("obsolete standalone key/auth UI still present: %s", forbidden)
		}
	}

	basePos := strings.Index(index, `id="base-url"`)
	keysPos := strings.Index(index, `id="keys-table"`)
	if basePos < 0 || keysPos < 0 || keysPos <= basePos {
		t.Fatalf("API key management must appear below Base URL: base=%d keys=%d", basePos, keysPos)
	}

	appData, err := embedded.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	app := string(appData)
	if !strings.Contains(app, `async function loadAPI(){$("#base-url").textContent=location.origin+"/v1";await Promise.all([loadKeys(),loadAPITestModels()]);}`) {
		t.Fatal("API page must load managed API keys and benchmark models")
	}
	if strings.Contains(app, `if(page==="keys")return loadKeys();`) {
		t.Fatal("standalone keys page routing should be removed")
	}
}


func TestAccountTableUsesPersistentEnableSwitch(t *testing.T) {
	appData, err := embedded.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	app := string(appData)
	for _, required := range []string{
		`data-enabled="`,
		`setAccountEnabled(`,
		`a.enabled!==false`,
	} {
		if !strings.Contains(app, required) {
			t.Fatalf("account enable switch behavior missing: %s", required)
		}
	}
	if strings.Contains(app, `data-toggle="`) {
		t.Fatal("legacy account enable/disable action button should be removed")
	}
}


func TestAPIBenchmarkControlsAndMetricsPresent(t *testing.T) {
	indexData, err := embedded.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	index := string(indexData)
	for _, required := range []string{
		`id="api-test-form"`,
		`id="api-test-model"`,
		`id="api-test-context"`,
		`id="api-test-concurrency"`,
		`id="api-test-output-tokens"`,
		`id="api-test-results"`,
	} {
		if !strings.Contains(index, required) {
			t.Fatalf("API benchmark control missing: %s", required)
		}
	}

	appData, err := embedded.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	app := string(appData)
	for _, required := range []string{
		`"/api/benchmark/chat"`,
		`context_window_tokens`,
		`throughput_tps`,
		`input_tokens`,
		`output_tokens`,
		`total_tokens`,
		`ttft_ms`,
		`duration_ms`,
	} {
		if !strings.Contains(app, required) {
			t.Fatalf("API benchmark metric missing: %s", required)
		}
	}
}
