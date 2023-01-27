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

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/util/metrics"
	log "github.com/sirupsen/logrus"
)

// Fetcher is used to query algod for new blocks.
type Fetcher interface {
	Algod() *algod.Client

	// go bot.Run()
	Run(ctx context.Context) error

	SetBlockHandler(f func(context.Context, *rpcs.EncodedBlockCert) error)
	SetNextRound(nextRound uint64)

	// Error returns any error fetcher is currently experiencing.
	Error() string
}

type fetcherImpl struct {
	algorandData string
	aclient      *algod.Client
	algodLastmod time.Time // newest mod time of algod.net algod.token

	handler func(context.Context, *rpcs.EncodedBlockCert) error

	nextRound uint64

	failingSince time.Time

	log *log.Logger

	err   error // protected by `errmu`
	errmu sync.Mutex

	// To improve performance, we fetch new blocks and call the block handler concurrently.
	// This queue contains the blocks that have been fetched but haven't been given to
	// the handler.
	blockQueue chan *rpcs.EncodedBlockCert
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

func (bot *fetcherImpl) setError(err error) {
	bot.errmu.Lock()
	bot.err = err
	bot.errmu.Unlock()
}

func (bot *fetcherImpl) processQueue(ctx context.Context) error {
	for {
		select {
		case block, ok := <-bot.blockQueue:
			if !ok {
				return nil
			}
			err := bot.handler(ctx, block)
			if err != nil {
				return fmt.Errorf("processQueue() handler err: %w", err)
			}
		case <-ctx.Done():
			return fmt.Errorf("processQueue: ctx.Err(): %w", ctx.Err())
		}
	}
}

func (bot *fetcherImpl) enqueueBlock(ctx context.Context, blockbytes []byte) error {
	block := new(rpcs.EncodedBlockCert)
	err := protocol.Decode(blockbytes, block)
	if err != nil {
		return fmt.Errorf("enqueueBlock() decode err: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case bot.blockQueue <- block:
		return nil
	}
}

// fetch the next block by round number until we find one missing (because it doesn't exist yet)
func (bot *fetcherImpl) catchupLoop(ctx context.Context) error {
	var err error
	var blockbytes []byte
	aclient := bot.Algod()
	for {
		start := time.Now()

		blockbytes, err = aclient.BlockRaw(bot.nextRound).Do(ctx)

		dt := time.Since(start)
		metrics.GetAlgodRawBlockTimeSeconds.Observe(dt.Seconds())

		if err != nil {
			// If context has expired.
			if ctx.Err() != nil {
				return fmt.Errorf("catchupLoop() fetch err: %w", err)
			}
			bot.log.WithError(err).Errorf("catchup block %d", bot.nextRound)
			return nil
		}

		err = bot.enqueueBlock(ctx, blockbytes)
		if err != nil {
			return fmt.Errorf("catchupLoop() err: %w", err)
		}
		// If we successfully handle the block, clear out any transient error which may have occurred.
		bot.setError(nil)
		bot.nextRound++
		bot.failingSince = time.Time{}
	}
}

// wait for algod to notify of a new round, then fetch that block
func (bot *fetcherImpl) followLoop(ctx context.Context) error {
	var err error
	var blockbytes []byte
	aclient := bot.Algod()
	for {
		for retries := 0; retries < 3; retries++ {
			// nextRound - 1 because the endpoint waits until "StatusAfterBlock"
			_, err = aclient.StatusAfterBlock(bot.nextRound - 1).Do(ctx)
			if err != nil {
				// If context has expired.
				if ctx.Err() != nil {
					return fmt.Errorf("followLoop() status err: %w", err)
				}
				bot.log.WithError(err).Errorf(
					"r=%d error getting status %d", retries, bot.nextRound)
				continue
			}
			start := time.Now()

			blockbytes, err = aclient.BlockRaw(bot.nextRound).Do(ctx)

			dt := time.Since(start)
			metrics.GetAlgodRawBlockTimeSeconds.Observe(dt.Seconds())

			if err == nil {
				break
			} else if ctx.Err() != nil { // if context has expired
				return fmt.Errorf("followLoop() fetch block err: %w", err)
			}
			bot.log.WithError(err).Errorf("r=%d err getting block %d", retries, bot.nextRound)
		}
		if err != nil {
			bot.setError(err)
			return nil
		}
		err = bot.enqueueBlock(ctx, blockbytes)
		if err != nil {
			return fmt.Errorf("followLoop() err: %w", err)
		}
		// Clear out any transient error which may have occurred.
		bot.setError(nil)
		bot.nextRound++
		bot.failingSince = time.Time{}
	}
}

func (bot *fetcherImpl) mainLoop(ctx context.Context) error {
	for {
		err := bot.catchupLoop(ctx)
		if err != nil {
			return fmt.Errorf("mainLoop() err: %w", err)
		}
		err = bot.followLoop(ctx)
		if err != nil {
			return fmt.Errorf("mainLoop() err: %w", err)
		}

		if bot.failingSince.IsZero() {
			bot.failingSince = time.Now()
		} else {
			now := time.Now()
			dt := now.Sub(bot.failingSince)
			bot.log.Warnf("failing to fetch from algod for %s, (since %s, now %s)", dt.String(), bot.failingSince.String(), now.String())
		}
		time.Sleep(5 * time.Second)
		err = bot.reclient()
		if err != nil {
			bot.setError(err)
			bot.log.WithError(err).Errorln("err trying to re-client")
		} else {
			bot.log.Infof("reclient happened")
		}
	}
}

// Run is part of the Fetcher interface
func (bot *fetcherImpl) Run(ctx context.Context) error {
	bot.blockQueue = make(chan *rpcs.EncodedBlockCert)

	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	ch0 := make(chan error, 1)
	go func() {
		ch0 <- bot.processQueue(ctx)
	}()

	ch1 := make(chan error, 1)
	go func() {
		ch1 <- bot.mainLoop(ctx)
	}()

	select {
	case err := <-ch0:
		cancelFunc()
		return fmt.Errorf("Run() err: %w", err)
	case err := <-ch1:
		cancelFunc()
		<-ch0
		return fmt.Errorf("Run() err: %w", err)
	}
}

// SetNextRound is part of the Fetcher interface
func (bot *fetcherImpl) SetNextRound(nextRound uint64) {
	bot.nextRound = nextRound
}

// AddBlockHandler is part of the Fetcher interface
func (bot *fetcherImpl) SetBlockHandler(handler func(context.Context, *rpcs.EncodedBlockCert) error) {
	bot.handler = handler
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
	netAddr, token, lastmod, err := AlgodArgsForDataDir(datadir)
	if err != nil {
		return
	}
	client, err = algod.MakeClient(netAddr, token)

	return
}

// AlgodArgsForDataDir opens the token and network files in the data directory, returning data for constructing client
func AlgodArgsForDataDir(datadir string) (netAddr string, token string, lastmod time.Time, err error) {
	netpath, tokenpath := algodPaths(datadir)
	var netaddrbytes []byte
	netaddrbytes, err = ioutil.ReadFile(netpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", netpath, err)
		return
	}
	netAddr = strings.TrimSpace(string(netaddrbytes))
	if !strings.HasPrefix(netAddr, "http") {
		netAddr = "http://" + netAddr
	}

	tokenBytes, err := ioutil.ReadFile(tokenpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", tokenpath, err)
		return
	}
	token = strings.TrimSpace(string(tokenBytes))

	if err == nil {
		lastmod, err = algodStat(netpath, tokenpath)
	}

	return
}
