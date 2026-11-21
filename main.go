// Command aumbry is the CLI entry point.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nerdneilsfield/aumbry/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := cmd.Execute(ctx)
	stop()
	if err != nil {
		var invalid *cmd.ValidationFailedError
		if errors.As(err, &invalid) {
			os.Exit(1) // the validation report is already on stdout
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
