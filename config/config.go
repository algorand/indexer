package config

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// EnvPrefix is the prefix for environment variable configurations.
const EnvPrefix = "INDEXER"

// FileTypes is an array of types of the config file.
var FileTypes = [...]string{"yml", "yaml"}

// FileName is the name of the config file. Don't use 'algorand-indexer', viper
// gets confused and thinks the binary is a config file with no extension.
const FileName = "indexer"

// BindFlagSet glues cobra and viper together via FlagSets
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
			_ = flags.Set(f.Name, fmt.Sprintf("%v", val))
		}
	})
}
