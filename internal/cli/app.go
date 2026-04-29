package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/vx6/vx6/internal/transfer"
)

func Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return errors.New("missing command")
	}

	switch args[0] {
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

func runSend(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	filePath := fs.String("file", "", "path to the file to send")
	address := fs.String("addr", "", "remote IPv6 address in [addr]:port form")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *filePath == "" {
		return errors.New("send requires --file")
	}
	if *address == "" {
		return errors.New("send requires --addr")
	}

	result, err := transfer.SendFile(ctx, *filePath, *address)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "sent %d bytes to %s\n", result.BytesSent, result.RemoteAddr)
	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "VX6")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  vx6 send --file <path> --addr [ipv6]:port")
}
