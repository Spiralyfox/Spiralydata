package main

import (
	"os"
	"path/filepath"
	"time"
)

func readFileWithRetry(path string) ([]byte, error) {
	var data []byte
	var err error

	for i := 0; i < 5; i++ {
		data, err = os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		if os.IsNotExist(err) {
			return nil, err
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil, err
}

func getExecutableDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exePath)
} 