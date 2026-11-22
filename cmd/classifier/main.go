// Command classifier serves the error classification API.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/app"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe /healthz of the local instance and exit")
	flag.Parse()

	if *healthcheck {
		os.Exit(probe())
	}

	os.Exit(serve())
}

func serve() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(os.Getenv("LOG_LEVEL"))}))

	if err := app.Run(ctx, os.Getenv, log); err != nil {
		log.Error("stopped", "err", err)

		return 1
	}

	return 0
}

func logLevel(name string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(name)); err != nil {
		return slog.LevelInfo
	}

	return l
}

// probe is used by the Docker HEALTHCHECK: the final image has no curl.
func probe() int {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bad LISTEN_ADDR:", err)

		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/healthz", http.NoBody) //nolint:gosec // loopback probe of our own port
	if err != nil {
		return 1
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // loopback probe of our own port
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)

		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 1
	}

	return 0
}
