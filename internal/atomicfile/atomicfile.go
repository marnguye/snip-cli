package atomicfile

import (
	"os"
	"path/filepath"
)

func Write(path string, data []byte) (err error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(temp)
		}
	}()
	if err = file.Chmod(0o600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp, path)
	}
	return err
}
