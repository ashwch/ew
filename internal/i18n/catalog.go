package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ashwch/ew/internal/appdirs"
)

type Catalog struct {
	Locale string        `json:"locale"`
	Loader LoaderCatalog `json:"loader"`
	Self   SelfCatalog   `json:"self"`
}

type LoaderCatalog struct {
	ThinkingFit []string `json:"thinking_fit"`
	Ranking     []string `json:"ranking"`
	History     []string `json:"history"`
	Debugging   []string `json:"debugging"`
	Default     []string `json:"default"`
}

type SelfCatalog struct {
	ShowConfig []string `json:"show_config"`
	SetupHooks []string `json:"setup_hooks"`
	Diagnose   []string `json:"diagnose"`
	Provider   []string `json:"provider"`
	UI         []string `json:"ui"`
	Mode       []string `json:"mode"`
	UIUpgrade  []string `json:"ui_upgrade"`
	Persist    []string `json:"persist"`
	Imperative []string `json:"imperative"`
	Question   []string `json:"question"`
}

func LoadCatalog(requestedLocale string) Catalog {
	locale := NormalizeLocale(requestedLocale)
	if locale == "" {
		locale = DetectLocale()
	}
	if locale == "" {
		locale = "en"
	}
	base := baseCatalogForLocale(locale)

	if override, ok := loadCommunityCatalog(locale); ok {
		merged := mergeCatalog(base, override)
		if strings.TrimSpace(override.Locale) != "" {
			merged.Locale = NormalizeLocale(override.Locale)
		} else {
			merged.Locale = locale
		}
		return merged
	}

	base.Locale = locale
	return base
}

func baseCatalogForLocale(locale string) Catalog {
	normalized := strings.ToLower(NormalizeLocale(locale))
	switch {
	case strings.HasPrefix(normalized, "hi"):
		// Hindi first, English fallback retained.
		base := mergeCatalog(defaultHindiCatalog(), defaultEnglishCatalog())
		base.Locale = "hi"
		return base
	default:
		base := defaultEnglishCatalog()
		base.Locale = "en"
		return base
	}
}

func DetectLocale() string {
	candidates := []string{
		os.Getenv("EW_LOCALE"),
		os.Getenv("LC_ALL"),
		os.Getenv("LC_MESSAGES"),
		os.Getenv("LANG"),
	}
	for _, candidate := range candidates {
		if normalized := NormalizeLocale(candidate); normalized != "" {
			return normalized
		}
	}
	return "en"
}

func NormalizeLocale(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.Split(trimmed, ".")[0]
	trimmed = strings.Split(trimmed, "@")[0]
	trimmed = strings.ReplaceAll(trimmed, "_", "-")

	parts := strings.Split(trimmed, "-")
	if len(parts) == 1 {
		lang := strings.ToLower(parts[0])
		if !isValidLocaleToken(lang, true) {
			return ""
		}
		return lang
	}
	lang := strings.ToLower(parts[0])
	region := strings.ToUpper(parts[1])
	if !isValidLocaleToken(lang, true) {
		return ""
	}
	if region == "" {
		return lang
	}
	if !isValidLocaleToken(strings.ToLower(region), false) {
		return ""
	}
	return lang + "-" + region
}

func isValidLocaleToken(token string, lettersOnly bool) bool {
	if len(token) < 2 || len(token) > 8 {
		return false
	}
	for _, r := range token {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if !lettersOnly && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func loadCommunityCatalog(locale string) (Catalog, bool) {
	configDir, err := appdirs.ConfigDir()
	if err != nil {
		return Catalog{}, false
	}

	normalized := NormalizeLocale(locale)
	if normalized == "" {
		return Catalog{}, false
	}
	lang := normalized
	if idx := strings.Index(lang, "-"); idx > 0 {
		lang = lang[:idx]
	}

	paths := []string{
		filepath.Join(configDir, "locales", normalized+".json"),
	}
	if lang != normalized {
		paths = append(paths, filepath.Join(configDir, "locales", lang+".json"))
	}

	for _, path := range paths {
		loaded, ok := loadCatalogFile(path)
		if ok {
			return loaded, true
		}
	}
	return Catalog{}, false
}

func loadCatalogFile(path string) (Catalog, bool) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return Catalog{}, false
	}
	var catalog Catalog
	if err := json.Unmarshal(bytes, &catalog); err != nil {
		return Catalog{}, false
	}
	return catalog, true
}

func mergeCatalog(base Catalog, override Catalog) Catalog {
	merged := base

	merged.Loader.ThinkingFit = mergeStringSlices(base.Loader.ThinkingFit, override.Loader.ThinkingFit)
	merged.Loader.Ranking = mergeStringSlices(base.Loader.Ranking, override.Loader.Ranking)
	merged.Loader.History = mergeStringSlices(base.Loader.History, override.Loader.History)
	merged.Loader.Debugging = mergeStringSlices(base.Loader.Debugging, override.Loader.Debugging)
	merged.Loader.Default = mergeStringSlices(base.Loader.Default, override.Loader.Default)

	merged.Self.ShowConfig = mergeStringSlices(base.Self.ShowConfig, override.Self.ShowConfig)
	merged.Self.SetupHooks = mergeStringSlices(base.Self.SetupHooks, override.Self.SetupHooks)
	merged.Self.Diagnose = mergeStringSlices(base.Self.Diagnose, override.Self.Diagnose)
	merged.Self.Provider = mergeStringSlices(base.Self.Provider, override.Self.Provider)
	merged.Self.UI = mergeStringSlices(base.Self.UI, override.Self.UI)
	merged.Self.Mode = mergeStringSlices(base.Self.Mode, override.Self.Mode)
	merged.Self.UIUpgrade = mergeStringSlices(base.Self.UIUpgrade, override.Self.UIUpgrade)
	merged.Self.Persist = mergeStringSlices(base.Self.Persist, override.Self.Persist)
	merged.Self.Imperative = mergeStringSlices(base.Self.Imperative, override.Self.Imperative)
	merged.Self.Question = mergeStringSlices(base.Self.Question, override.Self.Question)

	return merged
}

func mergeStringSlices(base []string, override []string) []string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	merged := make([]string, 0, len(base)+len(override))
	appendUnique := func(items []string) {
		for _, item := range items {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			if _, exists := seen[trimmed]; exists {
				continue
			}
			seen[trimmed] = struct{}{}
			merged = append(merged, trimmed)
		}
	}
	appendUnique(base)
	appendUnique(override)
	return merged
}

func defaultEnglishCatalog() Catalog {
	return Catalog{
		Locale: "en",
		Loader: LoaderCatalog{
			ThinkingFit: []string{
				"thinking of a command that fits",
				"thinking of a command that clicks",
				"thinking of a command that plays nice",
				"thinking of a command that just works",
				"thinking of a command that won't bite",
				"thinking of a command your future self approves",
				"thinking of a command with fewer regrets",
				"thinking of a command that keeps logs boring",
				"thinking of a command that avoids drama",
				"thinking of a command with sane defaults",
				"thinking of a command that keeps history clean",
				"thinking of a command that won't wake on-call",
				"thinking of a command that treats prod gently",
				"thinking of a command that survives copy/paste",
				"thinking of a command that respects your branch",
				"thinking of a command with less blast radius",
				"thinking of a command that keeps you moving",
				"thinking of a command that reads the room",
				"thinking of a command that feels obvious",
				"thinking of a command that leaves breadcrumbs",
				"thinking of a command that lands first try",
				"thinking of a command that does the right thing",
				"thinking of a command that won't surprise you",
				"thinking of a command that keeps momentum",
				"thinking of a command that is calmly correct",
				"thinking of a command that earns a rerun",
			},
			Ranking: []string{
				"ranking the best command",
				"ranking command candidates by safety first",
				"ranking commands by confidence and clarity",
				"ranking options for lowest blast radius",
				"ranking near matches and dropping noise",
				"ranking by what actually works in practice",
				"ranking by likely intent, not just keywords",
				"ranking commands that you can rerun safely",
				"ranking practical picks over clever ones",
				"ranking for the command you'd use twice",
			},
			History: []string{
				"scouting your history",
				"scouting your history and skipping noise",
				"scouting recent commands with intent in mind",
				"scouting shell history for practical matches",
				"scouting your history for reusable commands",
				"scouting your history and filtering false positives",
				"scouting history and favoring recent context",
				"scouting commands you actually ran",
				"scouting history for repeatable wins",
			},
			Debugging: []string{
				"debugging the failed command",
				"debugging by tracing the smallest safe fix",
				"debugging with reversible steps first",
				"debugging and avoiding side quests",
				"debugging toward one clean command",
				"debugging by checking common typo paths",
				"debugging with explicit command output in mind",
				"debugging while minimizing blast radius",
				"debugging to keep the next step obvious",
			},
			Default: []string{
				"{label}",
				"{label} (still cooking)",
				"{label} (almost there)",
				"{label} (double-checking details)",
				"{label} (staying out of your way)",
				"{label} (wrapping this up)",
				"{label} (polishing the final command)",
				"{label} (keeping it practical)",
			},
		},
		Self: SelfCatalog{
			ShowConfig: []string{
				"show config",
				"show settings",
				"my config",
				"my settings",
				"print config",
				"display config",
				"config_show",
			},
			SetupHooks: []string{
				"setup hooks",
				"set up hooks",
				"install hooks",
				"enable hooks",
				"hook snippet",
				"setup_hooks",
			},
			Diagnose: []string{
				"run doctor",
				"doctor",
				"diagnose ew",
				"health check",
				"check setup",
				"diagnose",
			},
			Provider: []string{
				"provider",
				"switch provider",
				"set provider",
				"use provider",
			},
			UI: []string{
				" ui ",
				" ui",
				"ui ",
				"backend",
				"interface",
			},
			Mode: []string{
				"mode",
			},
			UIUpgrade: []string{
				"switch",
				"change",
				"better",
				"best",
				"improve",
				"upgrade",
			},
			Persist: []string{
				" save",
				"save ",
				"persist",
				"remember",
				"default",
			},
			Imperative: []string{
				"switch",
				"change",
				"set ",
				"set to",
				"use ",
				"enable",
				"disable",
				"make ",
			},
			Question: []string{
				"?",
				"how ",
				"what ",
				"which ",
				"why ",
				"can ",
			},
		},
	}
}

func defaultHindiCatalog() Catalog {
	return Catalog{
		Locale: "hi",
		Loader: LoaderCatalog{
			ThinkingFit: []string{
				"ऐसा कमांड सोच रहा हूँ जो सही बैठे",
				"ऐसा कमांड सोच रहा हूँ जो तुरंत क्लिक करे",
				"ऐसा कमांड सोच रहा हूँ जो साफ-सुथरा चले",
				"ऐसा कमांड सोच रहा हूँ जो पहली बार में काम करे",
				"ऐसा कमांड सोच रहा हूँ जो अनावश्यक जोखिम न ले",
				"ऐसा कमांड सोच रहा हूँ जिसे आपका future-self approve करे",
				"ऐसा कमांड सोच रहा हूँ जिसमें पछतावा कम हो",
				"ऐसा कमांड सोच रहा हूँ जो logs को boring रखे",
				"ऐसा कमांड सोच रहा हूँ जो drama से दूर रहे",
				"ऐसा कमांड सोच रहा हूँ जिसमें sane defaults हों",
				"ऐसा कमांड सोच रहा हूँ जो history साफ रखे",
				"ऐसा कमांड सोच रहा हूँ जो on-call को न जगाए",
				"ऐसा कमांड सोच रहा हूँ जो prod को gently ट्रीट करे",
				"ऐसा कमांड सोच रहा हूँ जो copy/paste में न टूटे",
				"ऐसा कमांड सोच रहा हूँ जो branch context का सम्मान करे",
				"ऐसा कमांड सोच रहा हूँ जिसका blast radius कम हो",
				"ऐसा कमांड सोच रहा हूँ जो आपकी गति बनाए रखे",
				"ऐसा कमांड सोच रहा हूँ जो context पढ़कर चले",
				"ऐसा कमांड सोच रहा हूँ जो obvious महसूस हो",
				"ऐसा कमांड सोच रहा हूँ जो breadcrumbs छोड़ दे",
				"ऐसा कमांड सोच रहा हूँ जो first try में land करे",
				"ऐसा कमांड सोच रहा हूँ जो सही काम करे",
				"ऐसा कमांड सोच रहा हूँ जो surprise न दे",
				"ऐसा कमांड सोच रहा हूँ जो momentum बनाए रखे",
				"ऐसा कमांड सोच रहा हूँ जो calmly correct हो",
				"ऐसा कमांड सोच रहा हूँ जिसे दोबारा भी चलाना चाहें",
			},
			Ranking: []string{
				"सबसे अच्छे कमांड को rank कर रहा हूँ",
				"कमांड candidates को safety-first rank कर रहा हूँ",
				"confidence और clarity से rank कर रहा हूँ",
				"lowest blast radius वाले options ऊपर ला रहा हूँ",
				"near matches rank करके noise हटा रहा हूँ",
				"जो practically काम करे, उसे ऊपर रख रहा हूँ",
				"intent के हिसाब से rank कर रहा हूँ, सिर्फ keywords से नहीं",
				"ऐसे commands rank कर रहा हूँ जिन्हें safely rerun कर सकें",
				"clever tricks से ज़्यादा practical picks rank कर रहा हूँ",
				"वही command rank कर रहा हूँ जिसे आप दोबारा चलाएँ",
			},
			History: []string{
				"आपकी history स्कैन कर रहा हूँ",
				"history स्कैन करके noise छोड़ रहा हूँ",
				"recent commands को intent के साथ स्कैन कर रहा हूँ",
				"shell history से practical matches निकाल रहा हूँ",
				"reusable commands के लिए history छाँट रहा हूँ",
				"history स्कैन करके false positives हटा रहा हूँ",
				"recent context को प्राथमिकता देकर history पढ़ रहा हूँ",
				"वही commands ढूँढ रहा हूँ जो आपने सच में चलाए",
				"repeatable wins के लिए history छाँट रहा हूँ",
			},
			Debugging: []string{
				"failed command debug कर रहा हूँ",
				"सबसे छोटा safe fix trace कर रहा हूँ",
				"पहले reversible steps जाँच रहा हूँ",
				"side-quest से बचते हुए debug कर रहा हूँ",
				"एक clean command की तरफ debug कर रहा हूँ",
				"common typo paths पहले जाँच रहा हूँ",
				"explicit output देखकर debug कर रहा हूँ",
				"blast radius कम रखते हुए debug कर रहा हूँ",
				"अगला step obvious रहे, ऐसा debug कर रहा हूँ",
			},
			Default: []string{
				"{label}",
				"{label} (पक रहा है)",
				"{label} (लगभग हो गया)",
				"{label} (details दोबारा जाँच रहा हूँ)",
				"{label} (रास्ते से हटकर काम कर रहा हूँ)",
				"{label} (इसे wrap up कर रहा हूँ)",
				"{label} (final command polish कर रहा हूँ)",
				"{label} (practical ही रख रहा हूँ)",
			},
		},
		Self: SelfCatalog{
			ShowConfig: []string{
				"कॉन्फ़िग दिखाओ",
				"सेटिंग्स दिखाओ",
				"मेरी सेटिंग्स",
				"कॉन्फिगरेशन दिखाओ",
				"show config",
				"config_show",
			},
			SetupHooks: []string{
				"हुक सेटअप करो",
				"हुक इंस्टॉल करो",
				"हुक सक्षम करो",
				"hook snippet",
				"setup hooks",
				"setup_hooks",
			},
			Diagnose: []string{
				"डॉक्टर चलाओ",
				"डायग्नोज़",
				"स्वास्थ्य जांच",
				"सेटअप जांच",
				"doctor",
				"diagnose",
			},
			Provider: []string{
				"प्रोवाइडर",
				"provider",
				"set provider",
				"use provider",
			},
			UI: []string{
				"यूआई",
				"ui",
				"बैकएंड",
				"backend",
				"interface",
			},
			Mode: []string{
				"मोड",
				"mode",
			},
			UIUpgrade: []string{
				"बेहतर",
				"बेस्ट",
				"अपग्रेड",
				"बदलो",
				"बदलें",
				"switch",
				"change",
				"improve",
			},
			Persist: []string{
				"save",
				"persist",
				"remember",
				"default",
				"सहेज",
				"याद रखो",
				"डिफ़ॉल्ट",
			},
			Imperative: []string{
				"switch",
				"change",
				"set ",
				"use ",
				"enable",
				"disable",
				"make ",
				"बदलो",
				"बदलें",
				"सेट",
				"उपयोग",
			},
			Question: []string{
				"?",
				"how ",
				"what ",
				"which ",
				"why ",
				"can ",
				"कैसे",
				"क्या",
				"कौन",
				"क्यों",
			},
		},
	}
}
