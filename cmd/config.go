package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/nerdneilsfield/aumbry/internal/cliconfig"
)

func newConfigCommand() *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Manage CLI profiles"}
	command.AddCommand(newConfigSetCommand(), newConfigShowCommand(), newConfigUseCommand())
	return command
}

func newConfigSetCommand() *cobra.Command {
	var profileName, server, token string
	var dirs []string
	command := &cobra.Command{
		Use:   "set",
		Short: "Create or update a profile (only the given flags change)",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			flags := command.Flags()
			file, err := cliconfig.Load()
			if err != nil {
				return err
			}
			if profileName == "" {
				profileName = file.ActiveProfile
			}
			if profileName == "" {
				profileName = cliconfig.DefaultProfile
			}
			profile := file.Profiles[profileName]
			if flags.Changed("server") {
				profile.Server = server
			}
			if flags.Changed("token") {
				profile.Token = token
			}
			if flags.Changed("dir") {
				profile.DefaultDirs = dirs
			}
			if file.Profiles == nil {
				file.Profiles = map[string]cliconfig.Profile{}
			}
			file.Profiles[profileName] = profile
			if file.ActiveProfile == "" {
				file.ActiveProfile = profileName
			}
			if err = cliconfig.Save(file); err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "profile %q saved\n", profileName)
			return err
		},
	}
	flags := command.Flags()
	flags.StringVar(&profileName, "profile", "", "profile to modify (default: the active profile)")
	flags.StringVar(&server, "server", "", "server base URL")
	flags.StringVar(&token, "token", "", "API token")
	flags.StringArrayVar(&dirs, "dir", nil, "default directory ID (repeatable; replaces the existing list)")
	return command
}

func newConfigShowCommand() *cobra.Command {
	var profileName string
	command := &cobra.Command{
		Use:   "show",
		Short: "Print the effective profile (the token is masked)",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			file, err := cliconfig.Load()
			if err != nil {
				return err
			}
			name, profile, err := file.Resolve(profileName)
			if err != nil {
				return err
			}
			profile.Token = cliconfig.MaskToken(profile.Token)
			out, err := yaml.Marshal(struct {
				Name        string   `yaml:"profile"`
				Server      string   `yaml:"server"`
				Token       string   `yaml:"token"`
				DefaultDirs []string `yaml:"default_dirs"`
			}{name, profile.Server, profile.Token, profile.DefaultDirs})
			if err != nil {
				return err
			}
			_, err = command.OutOrStdout().Write(out)
			return err
		},
	}
	command.Flags().StringVar(&profileName, "profile", "", "profile to show (default: the active profile)")
	return command
}

func newConfigUseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "use <profile>",
		Short: "Switch the active profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			file, err := cliconfig.Load()
			if err != nil {
				return err
			}
			if _, ok := file.Profiles[args[0]]; !ok {
				return fmt.Errorf("unknown profile %q: create it with `aumbry config set --profile %s`", args[0], args[0])
			}
			file.ActiveProfile = args[0]
			if err = cliconfig.Save(file); err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "active profile: %s\n", args[0])
			return err
		},
	}
}
