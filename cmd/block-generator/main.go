package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/algorand/indexer/cmd/block-generator/generator"
	"github.com/algorand/indexer/util"
)

var configFile string
var port uint
var gen generator.Generator

const configFileName = "block_generator_config"

func init() {
	rand.Seed(12345)
	flag.StringVar(&configFile, "config", "", fmt.Sprintf("Override default config file from '%s'.", configFileName))
	flag.UintVar(&port, "port", 4010, "Port to start the server at.")

	viper.SetConfigName(configFileName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
}

func initializeConfigFile() (config generator.GenerationConfig, err error) {
	if len(configFile) > 0 {
		var f *os.File
		f, err = os.Open(configFile)
		if err != nil {
			return
		}

		err = viper.ReadConfig(f)
	} else {
		err = viper.ReadInConfig()
	}

	// Problem reading config
	if err != nil {
		return
	}

	err = viper.Unmarshal(&config)
	return
}

func main() {
	flag.Parse()

	config, err := initializeConfigFile()
	util.MaybeFail(err, "problem loading config file. Use '-config' or create a config file.")

	gen, err = generator.MakeGenerator(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create generator: %v", err)
		os.Exit(1)
	}

	http.HandleFunc("/", help)
	http.HandleFunc("/v2/blocks/", handleBlock)
	http.HandleFunc("/genesis", handleGenesis)

	portStr := fmt.Sprintf(":%d", port)
	fmt.Printf("Starting server at %s\n", portStr)
	http.ListenAndServe(portStr, nil)
}

func help(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Use /v2/blocks/:blocknum: to get a block.")
}

func handleGenesis(w http.ResponseWriter, r *http.Request) {
	gen.WriteGenesis(w)
}

func handleBlock(w http.ResponseWriter, r *http.Request) {
	// The generator doesn't actually care about the block...
	round, err := parseRound(r.URL.Path)
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}

	gen.WriteBlock(w, round)
}

const blockQueryPrefix = "/v2/blocks/"
const blockQueryBlockIdx = len(blockQueryPrefix)

func parseRound(path string) (uint64, error) {
	if !strings.HasPrefix(path, blockQueryPrefix) {
		return 0, fmt.Errorf("not a blocks query: %s", path)
	}

	result := uint64(0)
	pathlen := len(path)

	if pathlen == blockQueryBlockIdx {
		return 0, fmt.Errorf("no block in path")
	}

	for i := blockQueryBlockIdx; i < pathlen; i++ {
		if path[i] < '0' || path[i] > '9' {
			if i == blockQueryBlockIdx {
				return 0, fmt.Errorf("no block in path")
			}
			break
		}
		result = (uint64(10) * result) + uint64(int(path[i])-'0')
	}
	return result, nil
}
