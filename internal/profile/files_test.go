package profile

import (
	"reflect"
	"testing"
)

func TestFileValidationIgnoresRetiredModules(t *testing.T) {
	c := Default()
	EnsureFiles(&c)
	if err := ValidateFiles(c); err != nil {
		t.Fatal(err)
	}
	c.ActiveFile = ""
	if err := ValidateFiles(c); err == nil {
		t.Fatal("missing active profile accepted")
	}
	c.ActiveFile = "default"
	c.Files = append(c.Files, ConfigFile{ID: "two", Name: "default.conf", Content: "[Rule]\nFINAL,PROXY"})
	if err := ValidateFiles(c); err == nil {
		t.Fatal("duplicate filename accepted")
	}
	c.Files = c.Files[:1]
	for _, name := range []string{"../escape", "name\x00.conf", "..", "a/b.conf"} {
		if ValidFileName(name) {
			t.Fatal("invalid filename accepted")
		}
	}
	base, err := EffectiveRouting(c)
	if err != nil {
		t.Fatal(err)
	}
	c.Modules = []Module{{ID: "one", Name: "first", Enabled: true, Content: "[Rule]\nDOMAIN,lanboat.invalid,REJECT"}, {ID: "two", Name: "second", Enabled: true, Content: "[Rule]\nDOMAIN,lanboat.invalid,DIRECT"}}
	if err := ValidateFiles(c); err != nil {
		t.Fatal(err)
	}
	r, err := EffectiveRouting(c)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(base, r) {
		t.Fatal("retired modules changed routing rules")
	}
}
