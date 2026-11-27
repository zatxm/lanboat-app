package i18n

import "testing"

func TestDefaultLanguageSignals(t *testing.T) {
	for _, test := range []struct{ zone, locale, want string }{
		{"Asia/Shanghai", "en_US.UTF-8", "zh"}, {"Asia/Urumqi", "C", "zh"},
		{"Asia/Hong_Kong", "en_HK.UTF-8", "zh"}, {"Asia/Macao", "pt_MO.UTF-8", "zh"}, {"Asia/Taipei", "zh_TW.UTF-8", "zh"},
		{":/usr/share/zoneinfo/Asia/Shanghai", "C", "zh"}, {"PRC", "C", "zh"},
		{"America/New_York", "zh_CN.UTF-8", "en"}, {"Asia/Singapore", "zh_SG.UTF-8", "en"},
		{"UTC", "zh_CN.UTF-8", "zh"}, {"Etc/GMT-8", "en_US", "en"},
		{"", "zh-Hant-TW", "zh"}, {"invalid", "zh_HK.UTF-8", "zh"}, {"", "zh_SG.UTF-8", "en"}, {"", "C", "en"}, {"", "", "en"},
	} {
		t.Run(test.zone+"_"+test.locale, func(t *testing.T) {
			if got := detectLanguage(test.zone, test.locale); got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}
		})
	}
	t.Setenv("TZ", "UTC")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LC_MESSAGES", "zh_CN.UTF-8")
	t.Setenv("LANG", "zh_TW.UTF-8")
	if DefaultLanguage() != "en" {
		t.Fatal("LC_ALL precedence")
	}
	t.Setenv("LC_ALL", "")
	if DefaultLanguage() != "zh" {
		t.Fatal("LC_MESSAGES precedence")
	}
}

func TestMobileSystemLanguage(t *testing.T) {
	t.Cleanup(func() { platformLocale.Store(nil) })
	t.Setenv("LANG", "en_US.UTF-8")
	SetSystemLocale("Asia/Shanghai", "en-US")
	if DefaultLanguage() != "zh" {
		t.Fatal("mobile timezone should initialize the Chinese language")
	}
	SetSystemLocale("Etc/UTC", "zh-Hant")
	if DefaultLanguage() != "zh" {
		t.Fatal("mobile Chinese locale without a country should initialize the Chinese language")
	}
	SetSystemLocale("America/New_York", "en-US")
	if DefaultLanguage() != "en" {
		t.Fatal("mobile locale should initialize the English language")
	}
}
