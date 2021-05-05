package fetcher

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	log "github.com/sirupsen/logrus"
)

// Fetcher is used to query algod for new blocks.
type Fetcher interface {
	Algod() *algod.Client

	// go bot.Run()
	Run()

	AddBlockHandler(handler BlockHandler)
	SetContext(ctx context.Context)
	SetNextRound(nextRound uint64)

	// Error returns any error fetcher is currently experiencing.
	Error() string
}

// BlockHandler is the handler fetcher uses to process a block.
type BlockHandler interface {
	HandleBlock(block *rpcs.EncodedBlockCert)
}

type fetcherImpl struct {
	algorandData string
	aclient      *algod.Client
	algodLastmod time.Time // newest mod time of algod.net algod.token

	blockHandlers []BlockHandler

	nextRound uint64

	ctx  context.Context
	done bool

	failingSince time.Time

	log *log.Logger

	err   error // protected by `errmu`
	errmu sync.Mutex
}

func (bot *fetcherImpl) Error() string {
	bot.errmu.Lock()
	defer bot.errmu.Unlock()

	if bot.err != nil {
		return bot.err.Error()
	}
	return ""
}

// Algod is part of the Fetcher interface
func (bot *fetcherImpl) Algod() *algod.Client {
	return bot.aclient
}

func (bot *fetcherImpl) isDone() bool {
	if bot.done {
		return true
	}
	if bot.ctx == nil {
		return false
	}
	select {
	case <-bot.ctx.Done():
		bot.done = true
		return true
	default:
		return false
	}
}

func (bot *fetcherImpl) setError(err error) {
	bot.errmu.Lock()
	bot.err = err
	bot.errmu.Unlock()
}

// fetch the next block by round number until we find one missing (because it doesn't exist yet)
func (bot *fetcherImpl) catchupLoop() {
	var err error
	var blockbytes []byte
	aclient := bot.Algod()
	for true {
		if bot.isDone() {
			return
		}

		blockbytes, err = aclient.BlockRaw(bot.nextRound).Do(context.Background())
		if err != nil {
			bot.setError(err)
			bot.log.WithError(err).Errorf("catchup block %d", bot.nextRound)
			return
		}

		err = bot.handleBlockBytes(blockbytes)
		if err != nil {
			bot.setError(err)
			bot.log.WithError(err).Errorf("err handling catchup block %d", bot.nextRound)
			return
		}
		bot.nextRound++
		bot.failingSince = time.Time{}
	}
}

// wait for algod to notify of a new round, then fetch that block
func (bot *fetcherImpl) followLoop() {
	var err error
	var blockbytes []byte
	aclient := bot.Algod()
	for true {
		for retries := 0; retries < 3; retries++ {
			if bot.isDone() {
				return
			}
			_, err = aclient.StatusAfterBlock(bot.nextRound).Do(context.Background())
			if err != nil {
				bot.log.WithError(err).Errorf("r=%d error getting status %d", retries, bot.nextRound)
				continue
			}
			blockbytes, err = aclient.BlockRaw(bot.nextRound).Do(context.Background())
			if err == nil {
				break
			}
			bot.log.WithError(err).Errorf("r=%d err getting block %d", retries, bot.nextRound)
		}
		if err != nil {
			bot.setError(err)
			return
		}
		err = bot.handleBlockBytes(blockbytes)
		if err != nil {
			bot.setError(err)
			bot.log.WithError(err).Errorf("err handling follow block %d", bot.nextRound)
			break
		}
		// If we successfully handle the block, clear out any transient error which may have occurred.
		bot.setError(nil)
		bot.nextRound++
		bot.failingSince = time.Time{}
	}
}

// Run is part of the Fetcher interface
func (bot *fetcherImpl) Run() {
	for true {
		if bot.isDone() {
			return
		}
		bot.catchupLoop()
		bot.followLoop()
		if bot.isDone() {
			return
		}

		if bot.failingSince.IsZero() {
			bot.failingSince = time.Now()
		} else {
			now := time.Now()
			dt := now.Sub(bot.failingSince)
			bot.log.Warnf("failing to fetch from algod for %s, (since %s, now %s)", dt.String(), bot.failingSince.String(), now.String())
		}
		time.Sleep(5 * time.Second)
		err := bot.reclient()
		if err != nil {
			bot.setError(err)
			bot.log.WithError(err).Errorln("err trying to re-client")
		} else {
			bot.log.Infof("reclient happened")
		}
	}
}

// SetContext is part of the Fetcher interface
func (bot *fetcherImpl) SetContext(ctx context.Context) {
	bot.ctx = ctx
}

// SetNextRound is part of the Fetcher interface
func (bot *fetcherImpl) SetNextRound(nextRound uint64) {
	bot.nextRound = nextRound
}

func (bot *fetcherImpl) handleBlockBytes(blockbytes []byte) error {
	var block rpcs.EncodedBlockCert
	err := protocol.Decode(blockbytes, &block)
	if err != nil {
		return fmt.Errorf("unable to decode block: %v", err)
	}

	if block.Block.Round() != basics.Round(bot.nextRound) {
		return fmt.Errorf("expected round %d but got %d", bot.nextRound, block.Block.Round())
	}

	for _, handler := range bot.blockHandlers {
		handler.HandleBlock(&block)
	}

	return nil
}

// AddBlockHandler is part of the Fetcher interface
func (bot *fetcherImpl) AddBlockHandler(handler BlockHandler) {
	if bot.blockHandlers == nil {
		x := make([]BlockHandler, 1, 10)
		x[0] = handler
		bot.blockHandlers = x
		return
	}
	for _, oh := range bot.blockHandlers {
		if oh == handler {
			return
		}
	}
	bot.blockHandlers = append(bot.blockHandlers, handler)
}

// ForDataDir initializes Fetcher to read data from the data directory.
func ForDataDir(path string, log *log.Logger) (bot Fetcher, err error) {
	boti := &fetcherImpl{algorandData: path, log: log}
	err = boti.reclient()
	if err == nil {
		bot = boti
	}
	return
}

// ForNetAndToken initializes Fetch to read data from an algod REST endpoint.
func ForNetAndToken(netaddr, token string, log *log.Logger) (bot Fetcher, err error) {
	var client *algod.Client
	if !strings.HasPrefix(netaddr, "http") {
		netaddr = "http://" + netaddr
	}
	client, err = algod.MakeClient(netaddr, token)
	if err != nil {
		return
	}
	bot = &fetcherImpl{aclient: client, log: log}
	return
}

func (bot *fetcherImpl) reclient() (err error) {
	if bot.algorandData == "" {
		return nil
	}
	// If we know the algod data dir, re-read the algod.net and
	// algod.token files and make a new API client object.
	var nclient *algod.Client
	var lastmod time.Time
	nclient, lastmod, err = algodClientForDataDir(bot.algorandData)
	if err == nil {
		bot.aclient = nclient
		bot.algodLastmod = lastmod
	}
	return
}

func algodPaths(datadir string) (netpath, tokenpath string) {
	netpath = filepath.Join(datadir, "algod.net")
	tokenpath = filepath.Join(datadir, "algod.token")
	return
}

func algodStat(netpath, tokenpath string) (lastmod time.Time, err error) {
	nstat, err := os.Stat(netpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", netpath, err)
		return
	}
	tstat, err := os.Stat(tokenpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", tokenpath, err)
		return
	}
	if nstat.ModTime().Before(tstat.ModTime()) {
		lastmod = tstat.ModTime()
	}
	lastmod = nstat.ModTime()
	return
}

func algodClientForDataDir(datadir string) (client *algod.Client, lastmod time.Time, err error) {
	// TODO: move this to go-algorand-sdk
	netpath, tokenpath := algodPaths(datadir)
	var netaddrbytes []byte
	netaddrbytes, err = ioutil.ReadFile(netpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", netpath, err)
		return
	}
	netaddr := strings.TrimSpace(string(netaddrbytes))
	if !strings.HasPrefix(netaddr, "http") {
		netaddr = "http://" + netaddr
	}
	token, err := ioutil.ReadFile(tokenpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", tokenpath, err)
		return
	}
	client, err = algod.MakeClient(netaddr, strings.TrimSpace(string(token)))
	if err == nil {
		lastmod, err = algodStat(netpath, tokenpath)
	}
	return
}
