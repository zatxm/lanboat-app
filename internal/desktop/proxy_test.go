package desktop

import (
	"github.com/goccy/go-json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fakeProxy(t *testing.T) (SystemProxy, map[string]string, string) {
	t.Helper()
	dir := t.TempDir()
	state := filepath.Join(dir, "settings.json")
	original := map[string]string{"org.gnome.system.proxy/mode": "'auto'", "org.gnome.system.proxy/use-same-proxy": "true", "org.gnome.system.proxy/ignore-hosts": "['localhost']", "org.gnome.system.proxy.http/use-authentication": "true"}
	for _, protocol := range []string{"http", "https", "socks"} {
		original["org.gnome.system.proxy."+protocol+"/host"] = "'previous-proxy'"
		original["org.gnome.system.proxy."+protocol+"/port"] = "9191"
	}
	b, _ := json.Marshal(original)
	if err := os.WriteFile(state, b, 0600); err != nil {
		t.Fatal(err)
	}
	script := `#!/usr/bin/python3
import json,os,sys
p=os.environ['LANBOAT_GSETTINGS_FILE'];state=json.load(open(p));command,schema,key=sys.argv[1:4];name=schema+'/'+key
if command=='writable':print('true')
elif command=='get':print(state[name])
elif command=='set':
 if os.environ.get('LANBOAT_GSETTINGS_FAIL')=='1' and name=='org.gnome.system.proxy.https/port' and sys.argv[4]=='1080':sys.exit(1)
 state[name]=sys.argv[4];open(p,'w').write(json.dumps(state))
`
	if err := os.WriteFile(filepath.Join(dir, "gsettings"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	t.Setenv("LANBOAT_GSETTINGS_FILE", state)
	return SystemProxy{Path: filepath.Join(dir, "backup.json")}, original, state
}

func readProxy(t *testing.T, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]string
	if err := json.Unmarshal(b, &values); err != nil {
		t.Fatal(err)
	}
	return values
}

func TestSystemProxyRecovery(t *testing.T) {
	p, original, state := fakeProxy(t)
	if err := p.Enable(1080); err != nil {
		t.Fatal(err)
	}
	if readProxy(t, state)["org.gnome.system.proxy/mode"] != "'manual'" {
		t.Fatal("proxy not enabled")
	}
	info, _ := os.Stat(p.Path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("backup permissions")
	}
	// A new instance models recovery on the next application launch.
	if err := (SystemProxy{Path: p.Path}).Restore(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(readProxy(t, state), original) {
		t.Fatal("original settings not restored")
	}
	if err := p.Enable(1080); err != nil {
		t.Fatal(err)
	}
	settings := readProxy(t, state)
	settings["org.gnome.system.proxy.http/host"] = "'user-change'"
	b, _ := json.Marshal(settings)
	_ = os.WriteFile(state, b, 0600)
	if err := p.Restore(); err != nil {
		t.Fatal(err)
	}
	if readProxy(t, state)["org.gnome.system.proxy.http/host"] != "'user-change'" {
		t.Fatal("overwrote independent user change")
	}
}

func TestSystemProxyPartialFailureRollsBack(t *testing.T) {
	p, original, state := fakeProxy(t)
	t.Setenv("LANBOAT_GSETTINGS_FAIL", "1")
	if err := p.Enable(1080); err == nil {
		t.Fatal("expected failure")
	}
	if !reflect.DeepEqual(readProxy(t, state), original) {
		t.Fatal("partial update not rolled back")
	}
	if _, err := os.Stat(p.Path); !os.IsNotExist(err) {
		t.Fatal("backup retained after successful rollback")
	}
}

func TestRecoverProxyBeforeReadingDamagedConfig(t *testing.T) {
	p, original, state := fakeProxy(t)
	if err := p.Enable(1080); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(p.Path)
	if err := os.Rename(p.Path, filepath.Join(dir, "proxy-backup.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir, "", zap.NewNop(), &Logs{}); err == nil {
		t.Fatal("damaged config accepted")
	}
	if !reflect.DeepEqual(readProxy(t, state), original) {
		t.Fatal("proxy not recovered before config error")
	}
}
