package appmeta

import (
	"errors"
	"golang.org/x/sys/windows/registry"
	"path/filepath"
)

func installedDataDir() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Lanboat`, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer k.Close()
	dir, _, err := k.GetStringValue("DataDir")
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err == nil && !filepath.IsAbs(dir) {
		return "", errors.New("Lanboat DataDir must be absolute")
	}
	return dir, err
}
