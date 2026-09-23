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
	if !strings.Contains(app, `async function loadAPI(){$("#base-url").textContent=location.origin+"/v1";await loadKeys();}`) {
		t.Fatal("API page must load managed API keys")
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
