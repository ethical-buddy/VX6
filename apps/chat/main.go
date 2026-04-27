package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	fs := flag.NewFlagSet("vx6-chat", flag.ExitOnError)
	httpAddr := fs.String("http", "127.0.0.1:8088", "local web UI address")
	transportAddr := fs.String("transport", "127.0.0.1:8787", "local VX6 chat transport address")
	publishInterval := fs.Duration("publish-interval", 5*time.Minute, "how often to republish the chat service record")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "vx6-chat:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := Options{
		HTTPAddr:        *httpAddr,
		TransportAddr:   *transportAddr,
		PublishInterval: *publishInterval,
	}
	if err := Run(ctx, os.Stdout, opts); err != nil {
		fmt.Fprintln(os.Stderr, "vx6-chat:", err)
		os.Exit(1)
	}
}
