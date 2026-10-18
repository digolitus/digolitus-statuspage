// Command healthcheck probes the local API liveness endpoint. It exists
// because the distroless runtime image has no shell, curl or wget.
package main

import (
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		port = "8080"
	}
	path := "/healthz"
	if len(os.Args) > 1 && os.Args[1] == "ready" {
		path = "/readyz"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	// #nosec G107 -- loopback URL; port is validated as an integer above and path is a constant.
	resp, err := client.Get("http://127.0.0.1:" + port + path)
	if err != nil {
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
