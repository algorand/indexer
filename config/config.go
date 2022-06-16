package config

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// EnvPrefix is the prefix for environment variable configurations.
const EnvPrefix = "INDEXER"

// FileType is the type of the config file.
const FileType = "yml"

// FileName is the name of the config file. Don't use 'algorand-indexer', viper
// gets confused and thinks the binary is a config file with no extension.
const FileName = "indexer"

// ConfigPaths are the different locations that algorand-indexer should look for config files.
var ConfigPaths = [...]string{".", "$HOME", "$HOME/.algorand-indexer/", "$HOME/.config/algorand-indexer/", "/etc/algorand-indexer/"}

// BindFlags glues the cobra and viper libraries together.
func BindFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Environment variables can't have dashes in them, so bind them to their equivalent
		// keys with underscores
		// e.g. prefix=STING and --favorite-color is set to STING_FAVORITE_COLOR
		if strings.Contains(f.Name, "-") {
			envVarSuffix := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
			viper.BindEnv(f.Name, fmt.Sprintf("%s_%s", EnvPrefix, envVarSuffix))
		}

		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && viper.IsSet(f.Name) {
			val := viper.Get(f.Name)
			cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
		}
	})
}

func BindFlagSet(flags *pflag.FlagSet) {
	flags.VisitAll(func(f *pflag.Flag) {
		// Environment variables can't have dashes in them, so bind them to their equivalent
		// keys with underscores
		// e.g. prefix=STING and --favorite-color is set to STING_FAVORITE_COLOR
		if strings.Contains(f.Name, "-") {
			envVarSuffix := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
			viper.BindEnv(f.Name, fmt.Sprintf("%s_%s", EnvPrefix, envVarSuffix))
		}

		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && viper.IsSet(f.Name) {
			val := viper.Get(f.Name)
			flags.Set(f.Name, fmt.Sprintf("%v", val))
		}
	})
}
