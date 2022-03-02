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
	"sync"
	"time"

	"github.com/shx-project/sphinx/blockchain"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/config"
	"github.com/shx-project/sphinx/consensus"
	"github.com/shx-project/sphinx/consensus/prometheus"
	"github.com/shx-project/sphinx/event/sub"
	"github.com/shx-project/sphinx/network/p2p"
	"github.com/shx-project/sphinx/txpool"
)

const (
	softResponseLimit = 2 * 1024 * 1024 // Target maximum size of returned blocks, headers or node data.
	estHeaderRlpSize  = 500             // Approximate size of an RLP encoded block header

	forceSyncCycle = 10 * time.Second
	txChanSize     = 5000000
	// This is the target size for the packs of transactions sent by txsyncLoop.
	// A pack can get larger than this if a single transactions exceeds this size.
	txsyncPackSize = 100 * 1024
)

var (
	once         sync.Once
	syncInstance *SynCtrl
)

type DoneEvent struct{}
type StartEvent struct{}
type FailedEvent struct{ Err error }

type SynCtrl struct {
	fastSync  uint32 // Flag whether fast sync is enabled (gets disabled if we already have blocks)
	AcceptTxs uint32 // Flag whether we're considered synchronised (enables transaction processing)

	txpool      *txpool.TxPool
	chainconfig *config.ChainConfig
	maxPeers    int

	SubProtocols []p2p.Protocol

	newBlockMux   *sub.TypeMux
	txCh          chan bc.TxPreEvent
	txSub         sub.Subscription
	minedBlockSub *sub.TypeMuxSubscription

	// channels for fetcher, syncer, txsyncLoop
	newPeerCh   chan *p2p.Peer
	txsyncCh    chan *txsync
	quitSync    chan struct{}
	noMorePeers chan struct{}

	// wait group is used for graceful shutdowns during downloading
	// and processing
	wg sync.WaitGroup
}

// InstanceSynCtrl returns the singleton of SynCtrl.
func InstanceSynCtrl() *SynCtrl {
	once.Do(func() {
		i, err := newSynCtrl(&config.GetShxConfigInstance().BlockChain, config.GetShxConfigInstance().Node.SyncMode, txpool.GetTxPool(), prometheus.InstancePrometheus())
		if err != nil {
			log.Error("Failed to instance SynCtrl", "err", err)
		}
		syncInstance = i
	})
	return syncInstance
}

// NewSynCtrl returns a new block synchronization controller.
func newSynCtrl(cfg *config.ChainConfig, mode config.SyncMode, txpoolins *txpool.TxPool,
	engine consensus.Engine) (*SynCtrl, error) {
	synctrl := &SynCtrl{
		newBlockMux: new(sub.TypeMux),
		txpool:      txpoolins,
		chainconfig: cfg,
		newPeerCh:   make(chan *p2p.Peer),
		noMorePeers: make(chan struct{}),
		txsyncCh:    make(chan *txsync),
		quitSync:    make(chan struct{}),
	}

	p2p.PeerMgrInst().RegMsgProcess(p2p.TxMsg, HandleTxMsg)
	p2p.PeerMgrInst().RegMsgProcess(p2p.WorkProofMsg, HandleWorkProofMsg)
	p2p.PeerMgrInst().RegMsgProcess(p2p.ProofConfirmMsg, HandleProofConfirmMsg)

	p2p.PeerMgrInst().RegMsgProcess(p2p.GetStateMsg, HandleGetStateMsg)
	p2p.PeerMgrInst().RegMsgProcess(p2p.ResStateMsg, HandleResStateMsg)

	go TxsPoolLoop()
	return synctrl, nil
}

func (this *SynCtrl) NewBlockMux() *sub.TypeMux {
	return this.newBlockMux
}

func (this *SynCtrl) Start() {
	// broadcast transactions
	this.txCh = make(chan bc.TxPreEvent, txChanSize)
	this.txSub = this.txpool.SubscribeTxPreEvent(this.txCh)

	go this.txRoutingLoop()

	// broadcast mined blocks
	this.minedBlockSub = this.newBlockMux.Subscribe(bc.RoutWorkProofEvent{}, bc.RoutConfirmEvent{}, bc.RoutQueryStateEvent{}, bc.RoutResponseStateEvent{})
	go this.minedRoutingLoop()

	// start sync handlers
	go this.txsyncLoop()
}

// Mined routing loop
func (this *SynCtrl) minedRoutingLoop() {
	log.Debug("synctrl minedRoutingLoop enter")
	// automatically stops if unsubscribe
	for obj := range this.minedBlockSub.Chan() {
		// used to broadcast generate event.
		switch ev := obj.Data.(type) {
		case bc.RoutWorkProofEvent:
			go routProof(ev.ProofMsg)
		case bc.RoutConfirmEvent:
			log.Debug("controller got confirm event, and to rout.")
			go routProofConfirm(ev.ConfirmMsg)
		case bc.RoutQueryStateEvent:
			go routQueryState(ev.QsMsg)
		case bc.RoutResponseStateEvent:
			go routResponseState(ev.Rs)
		}
	}
}

func (this *SynCtrl) Stop() {
	this.txSub.Unsubscribe()         // quits txRoutingLoop
	this.minedBlockSub.Unsubscribe() // quits minedRoutingLoop

	// Wait for all peer handler goroutines and the loops to come down.
	this.wg.Wait()

	log.Info("Shx data sync stopped")
}
