// Copyright (C) 2019-2020 Algorand, Inc.
// This file is part of the Algorand Indexer
//
// Algorand Indexer is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// Algorand Indexer is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with Algorand Indexer.  If not, see <https://www.gnu.org/licenses/>.

package algobot

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/algorand/go-algorand-sdk/client/algod"
	"github.com/algorand/go-algorand-sdk/client/kmd"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"

	"github.com/algorand/indexer/types"
)

type Algobot interface {
	Algod() algod.Client
	Kmd() kmd.Client

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

type algobotImpl struct {
	algorandData string
	aclient      algod.Client

	kmdDir   string
	kmdUrl   string
	kmdToken string
	kclient  kmd.Client

	blockHandlers []BlockHandler

	nextRound uint64

	ctx context.Context
	wg  *sync.WaitGroup
}

func (bot *algobotImpl) Algod() algod.Client {
	return bot.aclient
}

func (bot *algobotImpl) Kmd() kmd.Client {
	// TODO: ensure kmd is running
	// TODO: lazy init of kclient
	return bot.kclient
}

func (bot *algobotImpl) isDone() bool {
	if bot.ctx == nil {
		return false
	}
	select {
	case <-bot.ctx.Done():
		return true
	default:
		return false
	}
}

func (bot *algobotImpl) Run() {
	if bot.wg != nil {
		defer bot.wg.Done()
	}
	var err error
	var blockbytes []byte
	aclient := bot.Algod()
	for true {
		if bot.isDone() {
			return
		}
		blockbytes, err = aclient.BlockRaw(bot.nextRound)
		if err != nil {
			log.Printf("first try block %d, err %v", bot.nextRound, err)
			break
		}
		err = bot.handleBlockBytes(blockbytes)
		if err != nil {
			log.Printf("err handling catchup block %d, ", bot.nextRound, err)
			return
		}
		bot.nextRound++
	}

	for true {
		for retries := 0; retries < 3; retries++ {
			if bot.isDone() {
				return
			}
			_, err = aclient.StatusAfterBlock(bot.nextRound)
			if err != nil {
				log.Printf("r=%d error getting status %d, %v", retries, bot.nextRound, err)
				continue
			}
			blockbytes, err = aclient.BlockRaw(bot.nextRound)
			if err == nil {
				log.Printf("r=%d success getting block %d, %v", retries, bot.nextRound, err)
				break
			}
			log.Printf("r=%d err getting block %d, %v", retries, bot.nextRound, err)
		}
		if err != nil {
			log.Printf("error getting block %d, %v", bot.nextRound, err)
			return
		}
		err = bot.handleBlockBytes(blockbytes)
		if err != nil {
			log.Printf("err handling follow block %d, ", bot.nextRound, err)
			return
		}
		bot.nextRound++
	}
}

func (bot *algobotImpl) SetWaitGroup(wg *sync.WaitGroup) {
	bot.wg = wg
}

func (bot *algobotImpl) SetContext(ctx context.Context) {
	bot.ctx = ctx
}

func (bot *algobotImpl) SetNextRound(nextRound uint64) {
	bot.nextRound = nextRound
}

func (bot *algobotImpl) handleBlockBytes(blockbytes []byte) (err error) {
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

func (bot *algobotImpl) AddBlockHandler(handler BlockHandler) {
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

func ForDataDir(path string) (bot Algobot, err error) {
	boti := &algobotImpl{algorandData: path}
	boti.aclient, err = algodClientForDataDir(path)
	if err == nil {
		bot = boti
	}
	return
}

func algodClientForDataDir(datadir string) (client algod.Client, err error) {
	// TODO: move this to go-algorand-sdk
	netpath := filepath.Join(datadir, "algod.net")
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
	tokenpath := filepath.Join(datadir, "algod.token")
	token, err := ioutil.ReadFile(tokenpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", tokenpath, err)
		return
	}
	client, err = algod.MakeClient(netaddr, strings.TrimSpace(string(token)))
	return
}

func kmdClientForDataDir(path string) (client kmd.Client, err error) {
	// TODO: WRITEME
	// TODO: use kmd in algod data dir if appropriate, otherwise ${HOME}/.algorand/kmd-v{N}
	// TODO: move this to go-algorand-sdk
	return kmd.Client{}, nil
}
