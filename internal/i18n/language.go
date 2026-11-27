package i18n

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

type localeInfo struct{ zone, language string }

var platformLocale atomic.Pointer[localeInfo]

// SetSystemLocale supplies information from mobile hosts that lack POSIX locale files.
func SetSystemLocale(zone, language string) {
	platformLocale.Store(&localeInfo{zone, language})
}

// DefaultLanguage uses a geographic timezone first, then the system locale.
// UTC and fixed offsets do not identify a country.
func DefaultLanguage() string {
	if locale := platformLocale.Load(); locale != nil {
		return detectLanguage(locale.zone, locale.language)
	}
	zone := strings.TrimPrefix(os.Getenv("TZ"), ":")
	if zone == "" {
		if path, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
			if _, tail, ok := strings.Cut(path, "zoneinfo/"); ok {
				zone = tail
			}
		}
	}
	if zone == "" {
		if data, err := os.ReadFile("/etc/timezone"); err == nil {
			zone = strings.TrimSpace(string(data))
		}
	}
	locale := os.Getenv("LC_ALL")
	if locale == "" {
		locale = os.Getenv("LC_MESSAGES")
	}
	if locale == "" {
		locale = os.Getenv("LANG")
	}
	return detectLanguage(zone, locale)
}

func detectLanguage(zone, locale string) string {
	zone = strings.TrimPrefix(strings.TrimPrefix(zone, ":"), "/usr/share/zoneinfo/")
	zone = strings.TrimPrefix(strings.TrimPrefix(zone, "posix/"), "right/")
	switch zone {
	case "Asia/Shanghai", "Asia/Chongqing", "Asia/Chungking", "Asia/Harbin", "Asia/Urumqi", "PRC", "Asia/Hong_Kong", "Hongkong", "Asia/Macau", "Asia/Macao", "Asia/Taipei", "ROC":
		return "zh"
	}
	if !strings.HasPrefix(zone, "Etc/") && zone != "UTC" && zone != "GMT" && zone != "Local" && zone != "" {
		if _, err := time.LoadLocation(zone); err == nil {
			return "en"
		}
	}
	locale = strings.ToUpper(strings.ReplaceAll(locale, "-", "_"))
	if parts := strings.FieldsFunc(locale, func(r rune) bool { return r == '.' || r == '@' || r == ':' }); len(parts) > 0 {
		locale = parts[0]
	}
	if locale == "ZH" || locale == "ZH_HANS" || locale == "ZH_HANT" {
		return "zh"
	}
	for _, part := range strings.Split(locale, "_") {
		switch part {
		case "CN", "HK", "MO", "TW":
			return "zh"
		}
	}
	return "en"
}
