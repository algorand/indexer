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
	f, err := os.Open(configFile)
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

// StartServer configures http handlers then runs ListanAndServe. Returns the http server and a done channel.
func StartServer(configFile string, addr string) (*http.Server, <- chan struct{}){
	config, err := initializeConfigFile(configFile)
	util.MaybeFail(err, "problem loading config file. Use '--config' or create a config file.")

	gen, err := MakeGenerator(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to make generator with config file '%s': %v", configFile, err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", help)
	mux.HandleFunc("/v2/blocks/", getBlockHandler(gen))
	mux.HandleFunc("/genesis", getGenesisHandler(gen))
	mux.HandleFunc("/report", getReportHandler(gen))

	srv := &http.Server{
		Addr: addr,
		Handler: mux,
	}

	done := make(chan struct{})
	go func() {
		defer close(done) // let main know we are done cleaning up

		// always returns error. ErrServerClosed on graceful close
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			// unexpected error. port in use?
			fmt.Errorf("ListenAndServe()\n: %v", err)
		}
	}()

	return srv, done
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
