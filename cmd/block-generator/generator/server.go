package generator

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/algorand/indexer/util"
)

func initializeConfigFile(configFile string) (config GenerationConfig, err error) {
	var f *os.File
	f, err = os.Open(configFile)
	if err != nil {
		return
	}

	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	err = viper.ReadConfig(f)

	// Problem reading config
	if err != nil {
		return
	}

	err = viper.Unmarshal(&config)
	return
}

// StartServer configures http handlers then runs ListanAndServe.
func StartServer(configFile string, port uint64) {
	config, err := initializeConfigFile(configFile)
	util.MaybeFail(err, "problem loading config file. Use '--config' or create a config file.")

	gen, err := MakeGenerator(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create generator: %v", err)
		os.Exit(1)
	}

	http.HandleFunc("/", help)
	http.HandleFunc("/v2/blocks/", getBlockHandler(gen))
	http.HandleFunc("/genesis", getGenesisHandler(gen))
	http.HandleFunc("/report", getReportHandler(gen))

	portStr := fmt.Sprintf(":%d", port)
	fmt.Printf("Starting server at %s\n", portStr)
	http.ListenAndServe(portStr, nil)
}

func help(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Use /v2/blocks/:blocknum: to get a block.")
}

func getReportHandler(gen Generator) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		gen.WriteReport(w)
	}
}

func getGenesisHandler(gen Generator) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		gen.WriteGenesis(w)
	}
}

func getBlockHandler(gen Generator) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// The generator doesn't actually care about the block...
		round, err := parseRound(r.URL.Path)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		gen.WriteBlock(w, round)
	}
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
