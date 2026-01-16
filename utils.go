package main

import (
	"os"
	"time"
)

// Lit un fichier avec retry pour Windows
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
