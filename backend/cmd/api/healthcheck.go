package main

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

func checkHealth(endpoint string) error {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}
