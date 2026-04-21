package cli

import (
	"io"
	"context"
	"flag"
	"errors"
	"fmt"
	"github.com/vx6/vx6/internal/config"
)

func runChat(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	peerName := fs.String("to", "", "peer name")
	address := fs.String("addr", "", "peer IPv6 address")
	configPath := fs.String("config", "", "path to VX6 config")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *peerName == "" && *address == "" {
		return errors.New("chat requires --to or --addr")
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}

	cfg, err := store.Load()
	if err != nil {
		return err
	}

	if *address == "" {
		peer, ok := cfg.Peers[*peerName]
		if !ok {
			return fmt.Errorf("unknown peer %q", *peerName)
		}

		*address = peer.Address
	}

	// later:
	// connect to *address
	// start chat session

	return nil
}