package main

import (
	"context"
	"fmt"
	"os"

	"github.com/vx6/vx6/internal/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "vx6:", err)
		os.Exit(1)
	}
}
