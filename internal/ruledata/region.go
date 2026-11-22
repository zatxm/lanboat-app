package ruledata

import (
	_ "embed"
	"github.com/goccy/go-json"
	"os"
	"strings"
	"time"
)

//go:embed assets/regions.json
var regionJSON []byte

type regionName struct {
	EN string `json:"en"`
	ZH string `json:"zh"`
}

var regions map[string]regionName

func init() { _ = json.Unmarshal(regionJSON, &regions) }
func Regions(language string) map[string]string {
	out := make(map[string]string, len(regions))
	for code, name := range regions {
		if language == "en" {
			out[code] = name.EN
		} else {
			out[code] = name.ZH
		}
	}
	return out
}
func ValidRegion(s string) bool { _, ok := regions[s]; return s == "" || ok }
func Region(selected string) string {
	if selected != "" {
		return selected
	}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		lang := os.Getenv(key)
		if lang == "" || lang == "C" || lang == "POSIX" {
			continue
		}
		lang = strings.SplitN(strings.SplitN(lang, ".", 2)[0], "@", 2)[0]
		parts := strings.FieldsFunc(lang, func(r rune) bool { return r == '_' || r == '-' })
		for _, part := range parts[1:] {
			code := strings.ToLower(part)
			if len(code) == 2 && ValidRegion(code) {
				return code
			}
		}
		break
	}
	zone := strings.TrimPrefix(os.Getenv("TZ"), ":")
	if zone == "" {
		if link, err := os.Readlink("/etc/localtime"); err == nil {
			if _, name, ok := strings.Cut(link, "/zoneinfo/"); ok {
				zone = name
			}
		}
	}
	if zone == "" {
		zone = time.Local.String()
	}
	if b, err := os.ReadFile("/usr/share/zoneinfo/zone.tab"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 3 && fields[2] == zone {
				code := strings.ToLower(fields[0])
				if ValidRegion(code) {
					return code
				}
			}
		}
	}
	return ""
}
