package i18n

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ashwch/ew/internal/appdirs"
)

func TestNormalizeLocale(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "en_US.UTF-8", want: "en-US"},
		{in: "es-ES", want: "es-ES"},
		{in: "fr", want: "fr"},
		{in: "pt_BR@latin", want: "pt-BR"},
		{in: "", want: ""},
	}
	for _, tc := range cases {
		if got := NormalizeLocale(tc.in); got != tc.want {
			t.Fatalf("NormalizeLocale(%q)=%q want=%q", tc.in, got, tc.want)
		}
	}
}

func TestLoadCatalogMergesCommunityOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("EW_LOCALE", "es-ES")

	configDir, err := appdirs.ConfigDir()
	if err != nil {
		t.Fatalf("config dir failed: %v", err)
	}
	localesDir := filepath.Join(configDir, "locales")
	if err := os.MkdirAll(localesDir, 0o755); err != nil {
		t.Fatalf("mkdir locales failed: %v", err)
	}

	override := `{
	  "locale": "es-ES",
	  "loader": {
	    "ranking": ["clasificando comandos con seguridad primero"]
	  },
	  "self": {
	    "show_config": ["mostrar configuracion"]
	  }
	}`
	if err := os.WriteFile(filepath.Join(localesDir, "es-ES.json"), []byte(override), 0o644); err != nil {
		t.Fatalf("write locale override failed: %v", err)
	}

	catalog := LoadCatalog("")
	if !strings.EqualFold(catalog.Locale, "es-ES") {
		t.Fatalf("expected merged locale es-ES, got %q", catalog.Locale)
	}
	if len(catalog.Loader.Ranking) == 0 {
		t.Fatalf("expected ranking messages")
	}
	foundSpanishRanking := false
	for _, msg := range catalog.Loader.Ranking {
		if strings.Contains(strings.ToLower(msg), "clasificando") {
			foundSpanishRanking = true
			break
		}
	}
	if !foundSpanishRanking {
		t.Fatalf("expected merged Spanish ranking message")
	}
	foundSpanishShowConfig := false
	for _, msg := range catalog.Self.ShowConfig {
		if strings.Contains(strings.ToLower(msg), "mostrar configuracion") {
			foundSpanishShowConfig = true
			break
		}
	}
	if !foundSpanishShowConfig {
		t.Fatalf("expected merged Spanish self-intent pattern")
	}
	// English fallback should remain present after merge.
	foundEnglish := false
	for _, msg := range catalog.Self.ShowConfig {
		if strings.Contains(strings.ToLower(msg), "show config") {
			foundEnglish = true
			break
		}
	}
	if !foundEnglish {
		t.Fatalf("expected english fallback pattern to remain available")
	}
}

func TestLoadCatalogHindiHasRichCoverage(t *testing.T) {
	catalog := LoadCatalog("hi-IN")
	if len(catalog.Loader.ThinkingFit) < 20 {
		t.Fatalf("expected rich Hindi thinking_fit coverage, got %d", len(catalog.Loader.ThinkingFit))
	}
	if len(catalog.Loader.Ranking) < 8 {
		t.Fatalf("expected rich Hindi ranking coverage, got %d", len(catalog.Loader.Ranking))
	}
	if len(catalog.Self.ShowConfig) < 4 {
		t.Fatalf("expected Hindi self-intent coverage for show config")
	}
}
