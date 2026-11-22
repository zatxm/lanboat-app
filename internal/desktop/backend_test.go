package desktop

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/zatxm/lanboat-app/internal/corebackend"
)

func init() {
	if len(os.Args) == 2 && os.Args[1] == "--lanboat-core" {
		if err := corebackend.Run(context.Background(), os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if testing.Testing() {
		os.Setenv("LANBOAT_CORE_TEST", "1")
	}
}
