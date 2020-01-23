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
	//"context"
	"errors"

	"github.com/algorand/go-algorand-sdk/client/algod"
	"github.com/algorand/go-algorand-sdk/client/kmd"
)

type Algobot interface {
	Algod() algod.Client
	Kmd() kmd.Client
}

type algobotImpl struct {
	algorandData string
	algodUrl     string
	algodToken   string
	aclient      algod.Client

	kmdDir   string
	kmdUrl   string
	kmdToken string
	kclient  kmd.Client
}

func (bot *algobotImpl) Algod() algod.Client {
	// TODO: lazy init of aclient
	return bot.aclient
}

func (bot *algobotImpl) Kmd() kmd.Client {
	// TODO: ensure kmd is running
	// TODO: lazy init of kclient
	return bot.kclient
}

func ForDataDir(path string) (bot Algobot, err error) {
	return nil, errors.New("Not Implemented")
}

func algodClientForDataDir(path string) (client algod.Client, err error) {
	// TODO: WRITEME
	// TODO: move this to go-algorand-sdk
	return algod.Client{}, nil
}

func kmdClientForDataDir(path string) (client kmd.Client, err error) {
	// TODO: WRITEME
	// TODO: use kmd in algod data dir if appropriate, otherwise ${HOME}/.algorand/kmd-v{N}
	// TODO: move this to go-algorand-sdk
	return kmd.Client{}, nil
}

/* TODO for general algobot
type TransactionHandler interface {
	HandleTransaction(Algobot) error
}
*/

type RawBlockHandler interface {
	HandleRawBlock(bot Algobot, blockbytes []byte) error
}
