package fetcher

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/algorand/go-algorand-sdk/client/algod"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"

	"github.com/algorand/indexer/types"
)

type Fetcher interface {
	Algod() algod.Client

	// go bot.Run()
	Run()

	AddBlockHandler(handler BlockHandler)
	SetWaitGroup(wg *sync.WaitGroup)
	SetContext(ctx context.Context)
	SetNextRound(nextRound uint64)
}

type BlockHandler interface {
	HandleBlock(block *types.EncodedBlockCert)
}

type fetcherImpl struct {
	algorandData string
	aclient      algod.Client
	algodLastmod time.Time // newest mod time of algod.net algod.token

	blockHandlers []BlockHandler

	nextRound uint64

	ctx  context.Context
	wg   *sync.WaitGroup
	done bool

	failingSince time.Time
}

func (bot *fetcherImpl) Algod() algod.Client {
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

// fetch the next block by round number until we find one missing (because it doesn't exist yet)
func (bot *fetcherImpl) catchupLoop() {
	var err error
	var blockbytes []byte
	aclient := bot.Algod()
	for true {
		if bot.isDone() {
			return
		}
		blockbytes, err = aclient.BlockRaw(bot.nextRound)
		if err != nil {
			log.Printf("catchup block %d, err %v\n", bot.nextRound, err)
			return
		}
		err = bot.handleBlockBytes(blockbytes)
		if err != nil {
			log.Printf("err handling catchup block %d, %v\n", bot.nextRound, err)
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
			_, err = aclient.StatusAfterBlock(bot.nextRound)
			if err != nil {
				log.Printf("r=%d error getting status %d, %v\n", retries, bot.nextRound, err)
				continue
			}
			blockbytes, err = aclient.BlockRaw(bot.nextRound)
			if err == nil {
				break
			}
			log.Printf("r=%d err getting block %d, %v\n", retries, bot.nextRound, err)
		}
		if err != nil {
			return
		}
		err = bot.handleBlockBytes(blockbytes)
		if err != nil {
			log.Printf("err handling follow block %d, %v\n", bot.nextRound, err)
			break
		}
		bot.nextRound++
		bot.failingSince = time.Time{}
	}
}

func (bot *fetcherImpl) Run() {
	if bot.wg != nil {
		defer bot.wg.Done()
	}
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
			log.Printf("failing to fetch from algod for %s, (since %s, now %s)\n", dt.String(), bot.failingSince.String(), now.String())
		}
		time.Sleep(5 * time.Second)
		err := bot.reclient()
		if err != nil {
			log.Printf("err trying to re-client, %v\n", err)
		} else {
			log.Print("reclient happened")
		}
	}
}

func (bot *fetcherImpl) SetWaitGroup(wg *sync.WaitGroup) {
	bot.wg = wg
}

func (bot *fetcherImpl) SetContext(ctx context.Context) {
	bot.ctx = ctx
}

func (bot *fetcherImpl) SetNextRound(nextRound uint64) {
	bot.nextRound = nextRound
}

func (bot *fetcherImpl) handleBlockBytes(blockbytes []byte) (err error) {
	var block types.EncodedBlockCert
	err = msgpack.Decode(blockbytes, &block)
	if err != nil {
		return
	}
	for _, handler := range bot.blockHandlers {
		handler.HandleBlock(&block)
	}
	return
}

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

func ForDataDir(path string) (bot Fetcher, err error) {
	boti := &fetcherImpl{algorandData: path}
	err = boti.reclient()
	if err == nil {
		bot = boti
	}
	return
}

func ForNetAndToken(netaddr, token string) (bot Fetcher, err error) {
	var client algod.Client
	if !strings.HasPrefix(netaddr, "http") {
		netaddr = "http://" + netaddr
	}
	client, err = algod.MakeClient(netaddr, token)
	if err != nil {
		return
	}
	bot = &fetcherImpl{aclient: client}
	return
}

func (bot *fetcherImpl) reclient() (err error) {
	if bot.algorandData == "" {
		return nil
	}
	// If we know the algod data dir, re-read the algod.net and
	// algod.token files and make a new API client object.
	var nclient algod.Client
	var lastmod time.Time
	nclient, lastmod, err = algodClientForDataDir(bot.algorandData)
	if err == nil {
		bot.aclient = nclient
		bot.algodLastmod = lastmod
	}
	return
}

func algodUpdated(datadir string) (lastmod time.Time, err error) {
	netpath, tokenpath := algodPaths(datadir)
	return algodStat(netpath, tokenpath)
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

func algodClientForDataDir(datadir string) (client algod.Client, lastmod time.Time, err error) {
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
