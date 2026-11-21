// Package cmd defines the Aumbry Cobra command tree.
package cmd

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/nerdneilsfield/aumbry/internal/logging"
)

type loggerKey struct{}

// NewRootCmd builds the Aumbry command tree.
func NewRootCmd() *cobra.Command {
	var verbose bool
	root := &cobra.Command{
		Use:           "aumbry",
		Short:         "Validate and publish Aumbry research documents",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(command *cobra.Command, _ []string) {
			ctx := context.WithValue(command.Context(), loggerKey{}, logging.New(verbose))
			command.SetContext(ctx)
		},
		PersistentPostRun: func(command *cobra.Command, _ []string) {
			_ = loggerFrom(command).Sync()
		},
	}
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")
	root.AddCommand(newVersionCommand(), newValidateCommand(), newConfigCommand(), newPushCommand())
	return root
}

// Execute runs the CLI with the supplied cancellation context.
func Execute(ctx context.Context) error {
	return NewRootCmd().ExecuteContext(ctx)
}

func loggerFrom(command *cobra.Command) *zap.Logger {
	if logger, ok := command.Context().Value(loggerKey{}).(*zap.Logger); ok {
		return logger
	}
	return zap.NewNop()
}

func newConfigCommand() *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Manage CLI profiles"}
	for _, name := range []string{"set", "show", "use"} {
		command.AddCommand(&cobra.Command{
			Use:   name,
			Short: "Manage the active profile",
			RunE: func(_ *cobra.Command, _ []string) error {
				return errors.New("config profiles are not implemented yet")
			},
		})
	}
	return command
}

func newPushCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "push <file.xml>",
		Short: "Upload or update a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("push is not implemented yet")
		},
	}
}
