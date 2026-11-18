package desktop

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatxm/lanboat-app/internal/routing"
)

func TestConfigImportPreviewDoesNotSave(t *testing.T) {
	a := testApp(t)
	content := "[Rule]\nDOMAIN,fixture.invalid,REJECT\nFINAL,DIRECT\n"
	file := filepath.Join(t.TempDir(), "rules.conf")
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rules":
			w.Write([]byte(content))
		case "/redirect":
			http.Redirect(w, r, "/rules", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/invalid":
			w.Write([]byte("[Rule]\nIP-CIDR,bad,PROXY"))
		case "/large":
			w.Write([]byte(strings.Repeat("a", routing.MaxConfig+1)))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	for _, input := range []string{content, file, server.URL + "/rules", server.URL + "/redirect"} {
		got, err := a.ParseConfigImport(input)
		if err != nil || strings.TrimSpace(got) != strings.TrimSpace(content) {
			t.Fatal("preview failed", input, err)
		}
	}
	for _, input := range []string{server.URL + "/invalid", server.URL + "/large", server.URL + "/missing", server.URL + "/loop", "[Rule]\nDOMAIN,invalid domain,PROXY"} {
		if _, err := a.ParseConfigImport(input); err == nil {
			t.Fatal("invalid preview accepted", input)
		}
	}
	if err := os.WriteFile(file, []byte("[Rule]\nDOMAIN,bad\xff,PROXY"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ParseConfigImport(file); err == nil {
		t.Fatal("non UTF-8 preview accepted")
	}
	tls := httptest.NewTLSServer(handler)
	defer tls.Close()
	if _, err := a.ParseConfigImport(tls.URL + "/rules"); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	a.client = tls.Client()
	if got, err := a.ParseConfigImport(tls.URL + "/rules"); err != nil || got != content {
		t.Fatal("HTTPS preview", err)
	}
	if c := a.GetState().Config; len(c.Files) != 1 || c.ActiveFile != "default" {
		t.Fatal("preview changed saved rules")
	}
}
