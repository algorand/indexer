package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/algorand/indexer/api"
	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/types"
)

var (
	algodDataDir     string
	algodAddr        string
	algodToken       string
	daemonServerAddr string
	noAlgod          bool
	developerMode    bool
	tokenString      string

	configFilePath string

	logger *log.Logger
)

func init() {
	logger = log.New()
	logger.SetFormatter(&log.JSONFormatter{})
	logger.SetOutput(os.Stdout)
	logger.SetLevel(log.InfoLevel)
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "run indexer daemon",
	Long:  "run indexer daemon. Serve api on HTTP.",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {
		if configFilePath != "" {
			cf, err := os.Open(configFilePath)
			maybeFail(err, "%s: %v", configFilePath, err)
			err = configFromStream(cf)
			maybeFail(err, "%s: %v", configFilePath, err)
			cf.Close()
		}
		if algodDataDir == "" {
			algodDataDir = os.Getenv("ALGORAND_DATA")
		}
		opts := idb.IndexerDbOptions{}
		if noAlgod {
			opts.ReadOnly = true
		}
		db := globalIndexerDb(&opts)

		ctx, cf := context.WithCancel(context.Background())
		defer cf()
		var bot fetcher.Fetcher
		var err error
		if noAlgod {
			fmt.Fprint(os.Stderr, "algod block following disabled\n")
		} else if algodAddr != "" && algodToken != "" {
			bot, err = fetcher.ForNetAndToken(algodAddr, algodToken)
			maybeFail(err, "fetcher setup, %v\n", err)
		} else if algodDataDir != "" {
			if genesisJsonPath == "" {
				genesisJsonPath = filepath.Join(algodDataDir, "genesis.json")
			}
			bot, err = fetcher.ForDataDir(algodDataDir)
			maybeFail(err, "fetcher setup, %v\n", err)
		} else {
			// no algod was found
			noAlgod = true
		}
		if !noAlgod {
			// Only do this if we're going to be writing
			// to the db, to allow for read-only query
			// servers that hit the db backend.
			err := importer.ImportProto(db)
			maybeFail(err, "import proto, %v\n", err)
		}
		if bot != nil {
			maxRound, err := db.GetMaxRound()
			if err == nil {
				bot.SetNextRound(maxRound + 1)
			}
			bih := blockImporterHandler{
				imp:   importer.NewDBImporter(db),
				db:    db,
				round: maxRound,
			}
			bot.AddBlockHandler(&bih)
			bot.SetContext(ctx)
			go func() {
				bot.Run()
				cf()
			}()
		}

		tokenArray := make([]string, 0)
		if tokenString != "" {
			tokenArray = append(tokenArray, tokenString)
		}

		// TODO: trap SIGTERM and call cf() to exit gracefully
		fmt.Printf("serving on %s\n", daemonServerAddr)
		api.Serve(ctx, daemonServerAddr, db, logger, tokenArray, developerMode)
	},
}

type configVar struct {
	name  string
	short string
	usage string
	t     configTypeVar
}
type configTypeVar interface {
	Set(string)
}
type configStringVar struct {
	value string
	ptr   *string
}

func (sv configStringVar) Set(x string) {
	// only set if it still has the original value
	if *sv.ptr == sv.value {
		*sv.ptr = x
	}
}

var configVars []configVar

func configStringVarP(flags *pflag.FlagSet, strPtr *string, name, short, value, usage string) {
	*strPtr = value
	flags.StringVarP(strPtr, name, short, value, usage)
	configVars = append(configVars, configVar{name, short, usage, &configStringVar{value, strPtr}})
}

// to-lower first
var trueStrings = []string{"t", "true", "1"}

type configBoolVar struct {
	value bool
	ptr   *bool
}

func (sv configBoolVar) Set(x string) {
	// only set if it still has the original value
	if *sv.ptr != sv.value {
		return
	}
	xl := strings.ToLower(x)
	for _, ts := range trueStrings {
		if ts == xl {
			*sv.ptr = true
			return
		}
	}
	*sv.ptr = false
}

func configBoolVarP(flags *pflag.FlagSet, boolPtr *bool, name, short string, value bool, usage string) {
	*boolPtr = value
	flags.BoolVarP(boolPtr, name, short, value, usage)
	configVars = append(configVars, configVar{name, short, usage, &configBoolVar{value, boolPtr}})
}

// TODO: maybe someday replace file parsing with YAML library, but for now we don't need nested structure and a smaller dependency tree makes me happy
func configFromStream(in io.Reader) (err error) {
	lineno := 0
	lineReader := bufio.NewReader(in)
	for true {
		linebytes, isPrefix, err := lineReader.ReadLine()
		if err == io.EOF {
			return nil
		}
		lineno++
		if err != nil {
			return err
		}
		if isPrefix {
			return fmt.Errorf(":%d line too long", lineno)
		}
		if len(linebytes) == 0 {
			continue
		}
		if linebytes[0] == '#' {
			continue
		}
		line := string(linebytes)
		colon := strings.IndexRune(line, ':')
		if colon < 0 {
			return fmt.Errorf(":%d line is not \"key: value\"", lineno)
		}
		key := strings.TrimSpace(line[:colon])
		value := strings.TrimSpace(line[colon+1:])
		ok := false
		for _, cvar := range configVars {
			if cvar.name == key {
				cvar.t.Set(value)
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf(":%d unknown key %s", lineno, key)
		}
	}
	return nil
}

func init() {
	configStringVarP(daemonCmd.Flags(), &algodDataDir, "algod", "d", "", "path to algod data dir, or $ALGORAND_DATA")
	configStringVarP(daemonCmd.Flags(), &algodAddr, "algod-net", "", "", "host:port of algod")
	configStringVarP(daemonCmd.Flags(), &algodToken, "algod-token", "", "", "api access token for algod")
	configStringVarP(daemonCmd.Flags(), &genesisJsonPath, "genesis", "g", "", "path to genesis.json (defaults to genesis.json in algod data dir if that was set)")
	configStringVarP(daemonCmd.Flags(), &daemonServerAddr, "server", "S", ":8980", "host:port to serve API on (default :8980)")
	configBoolVarP(daemonCmd.Flags(), &noAlgod, "no-algod", "", false, "disable connecting to algod for block following")
	configStringVarP(daemonCmd.Flags(), &tokenString, "token", "t", "", "an optional auth token, when set REST calls must use this token in a bearer format, or in a 'X-Indexer-API-Token' header")
	configBoolVarP(daemonCmd.Flags(), &developerMode, "dev-mode", "", false, "allow performance intensive operations like searching for accounts at a particular round")

	daemonCmd.Flags().StringVarP(&configFilePath, "config", "c", "", "path to 'key: value' config file, keys are same as command line options")

	// Make config entries for global flags
	configVars = append(configVars, configVar{"postgres", "P", "", &configStringVar{"", &postgresAddr}})
	configVars = append(configVars, configVar{"pidfile", "", "", &configStringVar{"", &pidFilePath}})
}

type blockImporterHandler struct {
	imp   importer.Importer
	db    idb.IndexerDb
	round uint64
}

func (bih *blockImporterHandler) HandleBlock(block *types.EncodedBlockCert) {
	start := time.Now()
	if uint64(block.Block.Round) != bih.round+1 {
		fmt.Fprintf(os.Stderr, "received block %d when expecting %d\n", block.Block.Round, bih.round+1)
	}
	bih.imp.ImportDecodedBlock(block)
	updateAccounting(bih.db)
	dt := time.Now().Sub(start)
	if len(block.Block.Payset) == 0 {
		// accounting won't have updated the round state, so we do it here
		stateJsonStr, err := db.GetMetastate("state")
		var state idb.ImportState
		if err == nil && stateJsonStr != "" {
			state, err = idb.ParseImportState(stateJsonStr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error parsing import state, %v\n", err)
				panic("error parsing import state in bih")
			}
		}
		state.AccountRound = int64(block.Block.Round)
		stateJsonStr = string(json.Encode(state))
		err = db.SetMetastate("state", stateJsonStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to save import state, %v\n", err)
		}
	}
	fmt.Printf("round r=%d (%d txn) imported in %s\n", block.Block.Round, len(block.Block.Payset), dt.String())
	bih.round = uint64(block.Block.Round)
}
