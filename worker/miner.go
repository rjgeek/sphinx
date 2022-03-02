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

// Package miner implements SHX block creation and mining.
package worker

import (
	"fmt"
	"github.com/shx-project/sphinx/blockchain/storage"
	"sync/atomic"

	"github.com/shx-project/sphinx/blockchain/state"
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/config"
	"github.com/shx-project/sphinx/consensus"
	"github.com/shx-project/sphinx/event/sub"
	"github.com/shx-project/sphinx/synctrl"
)

// Miner creates blocks and searches for proof-of-work values.
type Miner struct {
	mux *sub.TypeMux

	worker *worker

	coinbase       common.Address
	controlStarted bool
	mining         int32
	engine         consensus.Engine

	canStart    int32 // can start indicates whether we can start the mining operation
	shouldStart int32 // should start indicates whether we should start after sync
}

func New(config *config.ChainConfig, mux *sub.TypeMux, engine consensus.Engine, coinbase common.Address, db shxdb.Database) *Miner {
	miner := &Miner{
		mux:            mux,
		engine:         engine,
		worker:         newWorker(config, engine, coinbase, mux, db),
		canStart:       1,
		controlStarted: false,
	}
	return miner
}

// update keeps track of the synctrl events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
func (self *Miner) WorkControl() {
	events := self.mux.Subscribe(synctrl.StartEvent{}, synctrl.DoneEvent{}, synctrl.FailedEvent{})

	for ev := range events.Chan() {
		switch ev.Data.(type) {
		case synctrl.StartEvent:
			atomic.StoreInt32(&self.canStart, 0)
			if self.Mining() {
				self.Stop()
				atomic.StoreInt32(&self.shouldStart, 1)
				log.Info("Mining aborted due to sync")
			}
		case synctrl.DoneEvent, synctrl.FailedEvent:
			shouldStart := atomic.LoadInt32(&self.shouldStart) == 1

			atomic.StoreInt32(&self.canStart, 1)
			atomic.StoreInt32(&self.shouldStart, 0)
			if shouldStart {
				self.Start(self.coinbase)
			}
		}
	}
}

func (self *Miner) Start(coinbase common.Address) {
	if !self.controlStarted {
		self.controlStarted = true
		go self.WorkControl()
	}

	atomic.StoreInt32(&self.shouldStart, 1)
	self.worker.setShxerbase(coinbase)
	self.coinbase = coinbase

	if atomic.LoadInt32(&self.canStart) == 0 {
		log.Info("Network syncing, will start miner afterwards")
		return
	}
	atomic.StoreInt32(&self.mining, 1)

	log.Info("Starting mining operation")
	self.worker.start()
}

func (self *Miner) Stop() {
	self.worker.stop()
	atomic.StoreInt32(&self.mining, 0)
	atomic.StoreInt32(&self.shouldStart, 0)
	log.Info("Shx miner stoped")
}

func (self *Miner) SetOpt(maxtxs, peorid int) {
	self.worker.Setopt(maxtxs, peorid)
}

func (self *Miner) Mining() bool {
	return atomic.LoadInt32(&self.mining) > 0
}

func (self *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > config.MaximumExtraDataSize {
		return fmt.Errorf("Extra exceeds max length. %d > %v", len(extra), config.MaximumExtraDataSize)
	}
	self.worker.setExtra(extra)
	return nil
}

// Pending returns the currently pending block and associated state.
func (self *Miner) Pending() (*types.Block, *state.StateDB) {
	return self.worker.pending()
}

// PendingBlock returns the currently pending block.
//
// Note, to access both the pending block and the pending state
// simultaneously, please use Pending(), as the pending state can
// change between multiple method calls
func (self *Miner) PendingBlock() *types.Block {
	return self.worker.pendingBlock()
}
