// Package cmd defines the Aumbry Cobra command tree.
package cmd

import (
	"context"
	"io"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/nerdneilsfield/aumbry/internal/buildinfo"
	"github.com/nerdneilsfield/aumbry/internal/logging"
)

type loggerKey struct{}

// NewRootCmd builds the Aumbry command tree.
func NewRootCmd() *cobra.Command {
	var verbose bool
	var showVersion bool
	root := &cobra.Command{
		Use:           "aumbry",
		Short:         "Validate and publish Aumbry research documents",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if showVersion {
				_, err := io.WriteString(command.OutOrStdout(), buildinfo.Format("aumbry"))
				return err
			}
			return command.Help()
		},
		PersistentPreRun: func(command *cobra.Command, _ []string) {
			ctx := context.WithValue(command.Context(), loggerKey{}, logging.New(verbose))
			command.SetContext(ctx)
		},
		PersistentPostRun: func(command *cobra.Command, _ []string) {
			_ = loggerFrom(command).Sync()
		},
	}
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")
	root.Flags().BoolVar(&showVersion, "version", false, "print version information and exit")
	root.AddCommand(
		newVersionCommand(),
		newStatusCommand(),
		newValidateCommand(),
		newSchemaCommand(),
		newConfigCommand(),
		newPushCommand(),
		newDirsCommand(),
		newListCommand(),
		newUnpublishCommand(),
		newDeleteCommand(),
	)
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
