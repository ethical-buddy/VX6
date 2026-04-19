package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/vx6/vx6/internal/config"
	"github.com/vx6/vx6/internal/node"
	"github.com/vx6/vx6/internal/transfer"
)

func Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return errors.New("missing command")
	}

	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "node":
		return runNode(ctx, args[1:])
	case "peer":
		return runPeer(args[1:])
	case "send":
		return runSend(ctx, args[1:])
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	name := fs.String("name", "", "local human-readable node name")
	listenAddr := fs.String("listen", "[::]:4242", "default IPv6 listen address in [addr]:port form")
	dataDir := fs.String("data-dir", "./data/inbox", "default directory for received files")
	configPath := fs.String("config", "", "path to the VX6 config file")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("init requires --name")
	}
	if err := transfer.ValidateIPv6Address(*listenAddr); err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}

	cfg, err := store.Load()
	if err != nil {
		return err
	}
	cfg.Node.Name = *name
	cfg.Node.ListenAddr = *listenAddr
	cfg.Node.DataDir = *dataDir

	if err := store.Save(cfg); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "initialized node %q in %s\n", cfg.Node.Name, store.Path())
	return nil
}

func runSend(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	nodeName := fs.String("name", "", "local human-readable node name")
	filePath := fs.String("file", "", "path to the file to send")
	address := fs.String("addr", "", "remote IPv6 address in [addr]:port form")
	peerName := fs.String("to", "", "named peer from local VX6 config")
	configPath := fs.String("config", "", "path to the VX6 config file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *filePath == "" {
		return errors.New("send requires --file")
	}
	if *address == "" && *peerName == "" {
		return errors.New("send requires --addr or --to")
	}
	if *address != "" && *peerName != "" {
		return errors.New("send accepts only one of --addr or --to")
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}

	if *nodeName == "" {
		*nodeName = cfg.Node.Name
	}
	if *nodeName == "" {
		return errors.New("send requires --name or a configured node name via vx6 init")
	}

	if *peerName != "" {
		peer, err := store.ResolvePeer(*peerName)
		if err != nil {
			return err
		}
		*address = peer.Address
	}

	result, err := transfer.SendFile(ctx, transfer.SendRequest{
		NodeName: *nodeName,
		FilePath: *filePath,
		Address:  *address,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(
		os.Stdout,
		"sent %q (%d bytes) from node %q to %s\n",
		result.FileName,
		result.BytesSent,
		result.NodeName,
		result.RemoteAddr,
	)
	return nil
}

func runNode(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("node", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	nodeName := fs.String("name", "", "local human-readable node name")
	listenAddr := fs.String("listen", "", "IPv6 listen address in [addr]:port form")
	dataDir := fs.String("data-dir", "", "directory for received files")
	configPath := fs.String("config", "", "path to the VX6 config file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	cfgFile, err := store.Load()
	if err != nil {
		return err
	}

	if *nodeName == "" {
		*nodeName = cfgFile.Node.Name
	}
	if *listenAddr == "" {
		*listenAddr = cfgFile.Node.ListenAddr
	}
	if *dataDir == "" {
		*dataDir = cfgFile.Node.DataDir
	}
	if *nodeName == "" {
		return errors.New("node requires --name or a configured node name via vx6 init")
	}

	cfg := node.Config{
		Name:       *nodeName,
		ListenAddr: *listenAddr,
		DataDir:    *dataDir,
	}

	return node.Run(ctx, os.Stdout, cfg)
}

func runPeer(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return errors.New("missing peer subcommand")
	}

	switch args[0] {
	case "add":
		return runPeerAdd(args[1:])
	case "list":
		return runPeerList(args[1:])
	default:
		return fmt.Errorf("unknown peer subcommand %q", args[0])
	}
}

func runPeerAdd(args []string) error {
	fs := flag.NewFlagSet("peer add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	name := fs.String("name", "", "peer name")
	address := fs.String("addr", "", "peer IPv6 address in [addr]:port form")
	configPath := fs.String("config", "", "path to the VX6 config file")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("peer add requires --name")
	}
	if *address == "" {
		return errors.New("peer add requires --addr")
	}
	if err := transfer.ValidateIPv6Address(*address); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	if err := store.AddPeer(*name, *address); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "saved peer %q -> %s\n", *name, *address)
	return nil
}

func runPeerList(args []string) error {
	fs := flag.NewFlagSet("peer list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	configPath := fs.String("config", "", "path to the VX6 config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}

	names, peers, err := store.ListPeers()
	if err != nil {
		return err
	}

	for _, name := range names {
		fmt.Fprintf(os.Stdout, "%s\t%s\n", name, peers[name].Address)
	}
	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "VX6")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  vx6 init --name <node-name> [--listen [::]:4242] [--data-dir ./data/inbox]")
	fmt.Fprintln(w, "  vx6 node [--name <node-name>] [--listen [::]:4242] [--data-dir ./data/inbox]")
	fmt.Fprintln(w, "  vx6 peer add --name <peer-name> --addr [ipv6]:port")
	fmt.Fprintln(w, "  vx6 peer list")
	fmt.Fprintln(w, "  vx6 send [--name <node-name>] --file <path> (--addr [ipv6]:port | --to <peer-name>)")
}
