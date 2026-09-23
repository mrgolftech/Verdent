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
