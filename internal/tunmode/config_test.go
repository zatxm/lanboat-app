package tunmode

import (
	"context"
	"github.com/goccy/go-json"
	allinone "github.com/zatxm/lanboat-allinone"
	"os"
	"strings"
	"testing"
)

func TestTUNRequestAndCoreConfig(t *testing.T) {
	r := Request{DNSPort: 19091, ProxyPort: 1080, ControlPort: 19090, Secret: strings.Repeat("a", 32), OwnerPID: os.Getpid()}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Request{{ProxyPort: 22, ControlPort: 19090, Secret: r.Secret, OwnerPID: r.OwnerPID}, {ProxyPort: 1080, ControlPort: 1080, Secret: r.Secret, OwnerPID: r.OwnerPID}, {ProxyPort: 1080, ControlPort: 19090, Secret: r.Secret, OwnerPID: r.OwnerPID, ProxyUsername: strings.Repeat("x", 256)}} {
		if bad.Validate() == nil {
			t.Fatal("invalid privileged request accepted")
		}
	}
	raw, err := json.Marshal(r.Config("/opt/lanboat/lanboat", "native"))
	if err != nil {
		t.Fatal(err)
	}
	if err = allinone.Check(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
}
