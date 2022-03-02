// Copyright 2018 The sphinx Authors
// Modified based on go-ethereum, which Copyright (C) 2014 The go-ethereum Authors.
//
// The sphinx is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The sphinx is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the sphinx. If not, see <http://www.gnu.org/licenses/>.

package synctrl

import (
	"github.com/hashicorp/golang-lru"
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/network/p2p"
	"github.com/shx-project/sphinx/txpool"
	"gopkg.in/fatih/set.v0"
	"sync"
	"sync/atomic"
	"time"
)

var handleKnownBlocks = set.New()

var defaultRoutCount = int32(3)

type mulr struct {
	cache *lru.Cache
	mu    sync.Mutex
}

var (
	handleKnownProof        mulr
	handleKnownProofConfirm mulr
	handleResponseState     mulr
	handleQueryState        mulr
)

func init() {
	handleKnownProof.cache, _ = lru.New(100000)
	handleKnownProofConfirm.cache, _ = lru.New(100000)
	handleResponseState.cache, _ = lru.New(100000)
	handleQueryState.cache, _ = lru.New(100000)
}

var poolTxsCh chan *types.Transaction

func TxsPoolLoop() {
	duration := time.Millisecond * 500
	timer := time.NewTimer(duration)

	txCap := 2000
	txs := make([]*types.Transaction, 0, txCap)
	poolTxsCh = make(chan *types.Transaction, 2000000)

	for {
		select {
		case <-timer.C:
			if len(txs) > 0 {
				log.Debug("TxsPoolLoop timeout", "len(txs)", len(txs), "len(poolTxsCh)", len(poolTxsCh))
				go txpool.GetTxPool().AddTxs(txs)
				txs = make([]*types.Transaction, 0, txCap)
			}
		case tx, ok := <-poolTxsCh:
			if ok {
				txs = append(txs, tx)
				if len(txs) >= txCap {
					log.Debug("TxsPoolLoop full", "len(txs)", len(txs), "len(poolTxsCh)", len(poolTxsCh))
					go txpool.GetTxPool().AddTxs(txs)
					txs = make([]*types.Transaction, 0, txCap)
				}
			}

		}
		timer.Reset(duration)
	}
}

// HandleTxMsg deal received TxMsg
func HandleTxMsg(p *p2p.Peer, msg p2p.Msg) error {
	// Transactions arrived, make sure we have a valid and fresh chain to handle them
	// Don't change this code if you don't understand it
	if atomic.LoadUint32(&InstanceSynCtrl().AcceptTxs) == 0 {
		return nil
	}

	// Transactions can be processed, parse all of them and deliver to the pool
	var txs []*types.Transaction
	if err := msg.Decode(&txs); err != nil {
		return p2p.ErrResp(p2p.ErrDecode, "msg %v: %v", msg, err)
	}

	for i, tx := range txs {
		// Validate and mark the remote transaction
		if tx == nil {
			return p2p.ErrResp(p2p.ErrDecode, "transaction %d is nil", i)
		}
		p.KnownTxsAdd(tx.Hash())

		log.Debug("SHX profile", "Receive tx ", tx.Hash(), "at time ", time.Now().UnixNano()/1000/1000)
		if false { //txpool.GetTxPool().DupTx(tx) != nil {
			continue
		} else {
			go func() {
				tx.SetForward(true) // not need route to other peers.
				poolTxsCh <- tx
			}()
		}
	}

	return nil
}

func HandleWorkProofMsg(p *p2p.Peer, msg p2p.Msg) error {
	var proof types.WorkProofMsg
	if err := msg.Decode(&proof); err != nil {
		log.Error("Decode workproofmsg failed", "err", err)
		return p2p.ErrResp(p2p.ErrDecode, "msg %v: %v", msg, err)
	}
	{
		// broadcast proof max times.
		handleKnownProof.mu.Lock()
		defer handleKnownProof.mu.Unlock()
		var count int32
		if cache, ok := handleKnownProof.cache.Get(proof.Hash()); ok {
			count = cache.(int32)
		} else {
			count = defaultRoutCount
			// first time post to worker
			syncInstance.NewBlockMux().Post(proof)
		}
		if count > 0 {
			routProof(proof)
			count--
			handleKnownProof.cache.Add(proof.Hash(), count)
		}
	}

	return nil
}

func HandleProofConfirmMsg(p *p2p.Peer, msg p2p.Msg) error {
	var confirm types.ConfirmMsg
	if err := msg.Decode(&confirm); err != nil {
		return p2p.ErrResp(p2p.ErrDecode, "msg %v: %v", msg, err)
	}

	{
		// broadcast proof confirm max times.
		handleKnownProofConfirm.mu.Lock()
		defer handleKnownProofConfirm.mu.Unlock()
		var count int32
		if cache, ok := handleKnownProofConfirm.cache.Get(confirm.Hash()); ok {
			count = cache.(int32)
		} else {
			count = defaultRoutCount
			// first time post to worker
			syncInstance.NewBlockMux().Post(confirm)
		}
		if count > 0 {
			routProofConfirm(confirm)
			count--
			handleKnownProofConfirm.cache.Add(confirm.Hash(), count)
		}
	}

	return nil
}

func HandleResStateMsg(p *p2p.Peer, msg p2p.Msg) error {
	var response types.ResponseStateMsg
	if err := msg.Decode(&response); err != nil {
		return p2p.ErrResp(p2p.ErrDecode, "msg %v: %v", msg, err)
	}
	{
		// broadcast proof confirm max times.
		handleResponseState.mu.Lock()
		defer handleResponseState.mu.Unlock()
		var count int32
		if cache, ok := handleResponseState.cache.Get(response.Hash()); ok {
			count = cache.(int32)
		} else {
			count = defaultRoutCount
			// first time post to worker
			syncInstance.NewBlockMux().Post(response)
		}
		if count > 0 {
			routResponseState(response)
			count--
			handleResponseState.cache.Add(response.Hash(), count)
		}
	}

	return nil
}

func HandleGetStateMsg(p *p2p.Peer, msg p2p.Msg) error {
	var query types.QueryStateMsg
	if err := msg.Decode(&query); err != nil {
		log.Error("handlemsg QueryStateMsg decode failed", "err", err)
		return p2p.ErrResp(p2p.ErrDecode, "msg %v: %v", msg, err)
	}
	{
		// broadcast proof confirm max times.
		handleQueryState.mu.Lock()
		defer handleQueryState.mu.Unlock()
		var count int32
		if cache, ok := handleQueryState.cache.Get(query.Hash()); ok {
			count = cache.(int32)
		} else {
			count = defaultRoutCount
			// first time post to worker
			syncInstance.NewBlockMux().Post(query)
		}
		if count > 0 {
			routQueryState(query)
			count--
			handleQueryState.cache.Add(query.Hash(), count)
		}
	}
	return nil
}
