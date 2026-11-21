package cmd

import (
	"fmt"

	"github.com/nerdneilsfield/aumbry/internal/cliconfig"
)

func resolveProfile(profileName string) (cliconfig.Profile, error) {
	cfg, err := cliconfig.Load()
	if err != nil {
		return cliconfig.Profile{}, err
	}
	_, profile, err := cfg.Resolve(profileName)
	if err != nil {
		return cliconfig.Profile{}, err
	}
	if profile.Server == "" {
		return cliconfig.Profile{}, fmt.Errorf("no server configured: run `aumbry config set --server URL` or set %s", cliconfig.EnvServer)
	}
	if profile.Token == "" {
		return cliconfig.Profile{}, fmt.Errorf("no token configured: run `aumbry config set --token TOKEN` or set %s", cliconfig.EnvToken)
	}
	return profile, nil
}
