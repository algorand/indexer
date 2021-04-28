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
	flag.StringVar(&configFile, "config", "", "Override default config file.")
	flag.UintVar(&port, "port", 4010, "Port to start the server at.")

	viper.SetConfigName(configFileName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
}

func initializeConfigFile() error {
	var err error
	if len(configFile) > 0 {
		var f *os.File
		f, err = os.Open(configFile)
		if err != nil {
			return err
		}

		return viper.ReadConfig(f)
	}

	return viper.ReadInConfig()
}

func main() {
	flag.Parse()

	util.MaybeFail(initializeConfigFile(), "problem loading config file. Use '-config' or create a config file.")

	// Pass everything from the configuration into the generator.
	gen = generator.MakeGenerator(generator.GenerationConfig{
		TxnPerBlock:                  15000,
		NewAccountFrequency:          100,
		Protocol:                     "future",
		NumGenesisAccounts:           10,
		GenesisAccountInitialBalance: 1000000000,
		GenesisID:                    "blockgen-test",
		GenesisHash:                  [32]byte{},
	})

	portStr := fmt.Sprintf(":%d", port)

	http.HandleFunc("/", help)
	http.HandleFunc("/v2/blocks/", handleBlock)
	http.HandleFunc("/genesis", handleGenesis)

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
	//block, err := parseBlock(r.URL.Path)
	//if err != nil {
	//	fmt.Fprintf(w, err.Error())
	//	return
	//}

	gen.WriteBlock(w)
}

const blockQueryPrefix = "/v2/blocks/"
const blockQueryBlockIdx = len(blockQueryPrefix)

func parseBlock(path string) (uint64, error) {
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
		}
		result = (uint64(10) * result) + uint64(int(path[i])-'0')
	}
	return result, nil
}
