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

package worker

import (
	"github.com/hashicorp/golang-lru"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shx-project/sphinx/blockchain"
	"github.com/shx-project/sphinx/blockchain/state"
	"github.com/shx-project/sphinx/blockchain/storage"
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/config"
	"github.com/shx-project/sphinx/consensus"
	"github.com/shx-project/sphinx/event/sub"
	"github.com/shx-project/sphinx/txpool"
)

const (
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10
	waitConfirmTimeout = 30 // a proof wait confirm timeout seconds
)
var (
	blockMaxTxs = 5000 * 10
	minTxsToMine = 100000
	blockPeorid = 2
)

// Work is the workers current environment and holds
// all of the current state information
type Work struct {
	config *config.ChainConfig
	state     *state.StateDB 	// apply state changes here
	tcount    int            	// tx count in cycle

	Block *types.Block 			// the new block

	header   *types.Header
	txs      []*types.Transaction
	receipts []*types.Receipt
	states   []*types.ProofState
	createdAt time.Time
	id 			int64
	genCh  	chan error
	confirmed  bool
}

type RoundState byte

const (
	IDLE 		RoundState = iota
	PostMining
	Mining
)

// worker is the main object which takes care of applying messages to the new state
type worker struct {
	config *config.ChainConfig
	engine consensus.Engine
	mu sync.Mutex

	mux          *sub.TypeMux
	wg           sync.WaitGroup
	txpool       *txpool.TxPool
	chainHeadCh  chan bc.ChainHeadEvent
	chainHeadSub sub.Subscription

	confirmCh    chan *Work
	newRoundCh   chan *types.Header
	exitCh 		 chan struct {}

	chain   *bc.BlockChain
	chainDb shxdb.Database

	coinbase common.Address
	extra    []byte

	currentMu sync.Mutex
	current   *Work
	roundState 	atomic.Value

	unconfirm_mine *unconfirmedProofs // set of locally mined blocks pending canonicalness confirmations
	unconfirm_verify *unconfirmedProofs // set of locally verified and receive other's verify result.

	workPending	*WorkPending			// queue of confirmed block, write to blockchain.

	txMu			sync.Mutex
	txConfirmPool 	map[common.Hash]uint64
	updating 		bool
	history 		*lru.Cache
	verifyChMap     map[common.Address]chan types.WorkProofMsg
	peerLockMap     map[common.Address]chan struct{}

	// atomic status counters
	mining int32
}

func newWorker(config *config.ChainConfig, engine consensus.Engine, coinbase common.Address, mux *sub.TypeMux, db shxdb.Database) *worker {

	worker := &worker{
		config:      config,
		engine:      engine,
		mux:         mux,
		chainHeadCh: make(chan bc.ChainHeadEvent, chainHeadChanSize),
		chainDb:     db,
		confirmCh: 	 make(chan *Work, 10),
		chain:       bc.InstanceBlockChain(),
		coinbase:    coinbase,
		exitCh:		 make(chan struct {}),
		newRoundCh:  make(chan *types.Header, 1),
		txConfirmPool: make(map[common.Hash]uint64),
		updating:	false,
		workPending:NewWorkPending(),
		verifyChMap:make(map[common.Address]chan types.WorkProofMsg),
		peerLockMap:make(map[common.Address]chan struct{}),
	}
	worker.history,_ = lru.New(10)
	worker.roundState.Store(IDLE)

	worker.txpool = txpool.GetTxPool()
	worker.chainHeadSub = bc.InstanceBlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)
	worker.history.Add(worker.chain.CurrentHeader().ProofHash, struct {}{})

	blockPeorid = int(config.Prometheus.Period)
	// goto listen the event
	go worker.eventListener()
	go worker.workPending.Run()

	return worker
}

func (self *worker) setShxerbase(addr common.Address) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.coinbase = addr
}

func (self *worker) setExtra(extra []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.extra = extra
}

func (self *worker) Setopt(maxtxs,peorid int) {
	blockPeorid = peorid
	blockMaxTxs = maxtxs
}

func (self *worker) pending() (*types.Block, *state.StateDB) {
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	if atomic.LoadInt32(&self.mining) == 0 {
		return types.NewBlock(
			self.current.header,
			self.current.txs,
			self.current.states,
			self.current.receipts,
		), self.current.state.Copy()
	}
	return self.current.Block, self.current.state.Copy()
}

func (self *worker) pendingBlock() *types.Block {
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	if atomic.LoadInt32(&self.mining) == 0 {
		return types.NewBlock(
			self.current.header,
			self.current.txs,
			self.current.states,
			self.current.receipts,
		)
	}
	return self.current.Block
}

func (self *worker) start() {
	self.mu.Lock()
	defer self.mu.Unlock()
	if atomic.LoadInt32(&self.mining) == 1 {
		return
	}
	atomic.StoreInt32(&self.mining, 1)
	go self.RoutineMine()
}

func (self *worker) stop() {
	self.mu.Lock()
	defer self.mu.Unlock()
	if atomic.LoadInt32(&self.mining) == 0 {
		return
	}
	self.exitCh <- struct{}{}
	self.wg.Wait()
	atomic.StoreInt32(&self.mining, 0)
}

func (self *worker) eventListener() {
	events := self.mux.Subscribe(types.WorkProofMsg{}, types.QueryStateMsg{}, types.ResponseStateMsg{})
	defer events.Unsubscribe()

	defer self.chainHeadSub.Unsubscribe()

	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		// A real event arrived, process interesting content
		select {
		// Handle ChainHeadEvent
		case <-self.chainHeadCh:
			//Todo: self.PreMiner()
		case <-self.chainHeadSub.Err():
			//return

		case <-timer.C:
			if !self.updating {
				go self.updateTxConfirm()
			}
			timer.Reset(time.Second)

		case obj := <-events.Chan():
			switch ev:= obj.Data.(type) {
			case types.WorkProofMsg:
				sender, e := self.engine.RecoverSender(ev.Proof.Data(), ev.Sign)
				if e != nil {
					log.Debug("worker recover sender failed", "err ", e)
				} else {
					// sorted handle workproofevent for every peer.
					log.Debug("worker got workproof event", "from", sender)
					if ch,ok := self.verifyChMap[sender]; ok {
						ch <- ev
					} else {
						ch := make(chan types.WorkProofMsg, 1000)
						self.verifyChMap[sender] = ch
						ch <- ev
						go func(ch chan types.WorkProofMsg, sender common.Address) {
							for {
								select {
								case event,ok := <-ch:
									if !ok {
										return
									} else {
										self.dealProofEvent(&event, sender)
									}
								}
							}
						}(ch, sender)
					}
				}
			case types.QueryStateMsg:
				sender, e := self.engine.RecoverSender(ev.Qs.Data(), ev.Sign)
				if e != nil {
					log.Debug("worker recover sender failed", "err ", e)
				} else {
					log.Debug("worker got QueryState event", "from", sender)
					self.dealQueryState(&ev, sender)
				}
			case types.ResponseStateMsg:
				sender, e := self.engine.RecoverSender(ev.Rs.Data(), ev.Sign)
				if e != nil {
					log.Debug("worker recover sender failed", "err ", e)
				} else {
					log.Debug("worker got ResponseState event", "from", sender)
					self.dealResponseState(&ev, sender)
				}
			}
		}
	}
}
