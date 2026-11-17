package themes_test

import (
	"testing"

	"github.com/Dumhouse/homepage/internal/themes"
)

func TestRepoThemes(t *testing.T) {
	if err := themes.ValidateDir("../../themes"); err != nil {
		t.Fatalf("theme check: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	bad := []string{
		`{"name":"x"}`,
		`{"id":"x","name":"x","css":"@import url(https://evil)"}`,
		`{"id":"x","name":"x","modes":{"dark":{"bg":"#000"}}}`,
		`{"id":"x","name":"x","modes":{"dark":{"--bg":"#ffffff","--text":"#eeeeee"}}}`,
		`{"id":"x","name":"x","radius":99}`,
	}
	for i, raw := range bad {
		if err := themes.Validate([]byte(raw)); err == nil {
			t.Errorf("case %d should fail", i)
		}
	}
}
