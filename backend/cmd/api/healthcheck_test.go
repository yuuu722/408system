package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthcheckDistinguishesReadyAndFailedAPI(t *testing.T) {
	for _, status := range []int{200, 503} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		err := checkHealth(srv.URL)
		if (err == nil) != (status == 200) {
			t.Fatalf("HTTP %d: %v", status, err)
		}
		srv.Close()
		if checkHealth(srv.URL) == nil {
			t.Fatal("stopped API accepted as healthy")
		}
	}
}
