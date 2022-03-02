package worker

import (
	"encoding/hex"
	"errors"
	"github.com/shx-project/sphinx/blockchain"
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/consensus"
	"github.com/shx-project/sphinx/network/p2p"
	"github.com/shx-project/sphinx/network/p2p/discover"
	"github.com/shx-project/sphinx/txpool"
	"math/big"
	"time"
)

// makeCurrent creates a new environment for the current cycle.
func (self *worker) makeCurrent(parent *types.Header, header *types.Header) error {
	state, err := self.chain.StateAt(parent.Root)
	if err != nil {
		return err
	}
	work := &Work{
		config:    self.config,
		state:     state,
		header:    header,
		states:    make([]*types.ProofState, 0),
		createdAt: time.Now(),
		id:        time.Now().UnixNano(),
		genCh:     make(chan error, 1),
		confirmed: false,
	}

	peers := p2p.PeerMgrInst().PeersAll()
	for _, peer := range peers {
		if peer.RemoteType() == discover.MineNode {
			// Get all peers' proof state from db.
			proofState := bc.GetPeerProof(self.chainDb, peer.Address())
			if proofState != nil {
				work.states = append(work.states, proofState)
			}
		}
	}

	// Keep track of transactions which return errors so they can be removed
	work.tcount = 0
	self.current = work
	return nil
}

func (self *worker) CheckNeedStartMine() *types.Header {
	var head *types.Header
	if self.workPending.HaveErr() {
		if self.workPending.Empty() {
			// reset to no error, and continue to mine.
			self.workPending.SetNoError()
		} else {
			return nil
		}
	}
	if h := self.workPending.Top(); h != nil {
		head = h.Block.Header()
		log.Debug("worker get header from workpending", "h.hash", head.Hash(), "head.Number", head.Number,
			"head.ProofHash", hex.EncodeToString(head.ProofHash[:]))
	} else {
		head = self.chain.CurrentHeader()
		log.Debug("worker get header from self.chain", "h.hash", head.Hash(), "head.Number", head.Number,
			"head.ProofHash", hex.EncodeToString(head.ProofHash[:]))
	}

	now := time.Now().UnixNano() / 1000 / 1000
	pending, _ := self.txpool.Pended()
	delta := now - head.Time.Int64()
	if delta >= int64(blockPeorid*1000) || (len(pending) >= minTxsToMine) && delta > 20 {
		return head
	}
	return nil
}

func (self *worker) getRoundState() RoundState {
	v := self.roundState.Load().(RoundState)
	return v
}

func (self *worker) setRoundState(s RoundState) {
	self.roundState.Store(s)
}

func (self *worker) RoutineMine() {
	events := self.mux.Subscribe(types.ConfirmMsg{})
	defer events.Unsubscribe()

	self.confirmCh = make(chan *Work)
	self.newRoundCh = make(chan *types.Header)
	self.unconfirm_mine = newUnconfirmedProofs(self.confirmCh)
	go self.unconfirm_mine.RoutineLoop()

	go func() {
		// routine to check new mine round and start new mine round
		evict := time.NewTicker(time.Millisecond * 10)
		defer evict.Stop()
		for {
			select {
			case <-evict.C:
				log.Trace("worker routine check new round")
				if self.getRoundState() == IDLE {
					if h := self.CheckNeedStartMine(); h != nil {
						self.setRoundState(PostMining)
						go func() {
							defer func() {
								if err := recover(); err != nil {
									log.Debug("error on newRoundCh", "err", err)
								}
							}()
							self.newRoundCh <- h
						}()
					}
				} else if self.getRoundState() == Mining {
					// working is mining.
				}
			case lastHeader, ok := <-self.newRoundCh:
				if !ok {
					return
				}
				log.Info("worker routine start new round ", "time ", time.Now().UnixNano()/1000/1000)
				self.setRoundState(Mining)
				self.wg.Add(1)
				go func() {
					defer self.wg.Done()
					if err := self.NewMineRound(lastHeader); err != nil {
						self.setRoundState(IDLE)
					} else {
						self.setRoundState(Mining)
					}
				}()
			}
		}
	}()

	for {
		select {
		case obj := <-events.Chan():
			switch ev := obj.Data.(type) {
			case types.ConfirmMsg:
				sender, e := self.engine.RecoverSender(ev.Confirm.Data(), ev.Sign)
				if e != nil {
					log.Debug("worker got confirmEvent, but recover sender failed", "err", e)
				} else if sender != self.coinbase {
					log.Debug("worker got confirmMsg ", "from", sender)
					self.dealConfirm(&ev, sender)
				}
			}
		case work := <-self.confirmCh:
			self.wg.Add(1)
			go func() {
				defer self.wg.Done()
				log.Debug("worker start to exec finalMine", "time ", time.Now().UnixNano()/1000/1000)
				err := self.FinalMine(work)
				if err != nil {
					log.Debug("worker finalmine failed", "err ", err)
				}
				self.setRoundState(IDLE)
			}()

		case <-self.exitCh:
			self.unconfirm_mine.Stop()
			close(self.confirmCh)
			close(self.newRoundCh)
			self.setRoundState(IDLE)
			return
		}
	}
}

func (self *worker) NewMineRound(parent *types.Header) error {
	if p2p.PeerMgrInst().GetLocalType() == discover.BootNode {
		return nil
	}

	// make header
	if parent == nil {
		parent = self.chain.CurrentHeader()
	}
	num := big.NewInt(0).Add(parent.Number, common.Big1)
	header := &types.Header{
		ParentHash: parent.Hash(),
		Coinbase:   self.coinbase,
		Number:     num,
		Extra:      self.extra,
	}
	// prepare header
	pstate, _ := self.chain.StateAt(parent.Root)
	if err := self.engine.PrepareBlockHeader(self.chain, header, pstate); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return err
	}
	log.Debug("worker after prepareblock header", "time ", time.Now().UnixNano()/1000/1000)

	// make work
	err := self.makeCurrent(parent, header)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return err
	}

	txs := txpool.GetTxPool().Pending(self.current.id, blockMaxTxs)
	// Create the current work task and check any fork transitions needed
	work := self.current
	work.commitTransactions(txs, self.coinbase)

	log.Info("luxqdebug", "total work.txs ", len(work.txs), "total pending txs", len(txs), "time ", time.Now().UnixNano()/1000/1000)

	// generate workproof
	proof, err := self.engine.GenerateProof(self.chain, self.current.header, parent, work.txs, work.states)
	if err != nil {
		log.Error("Premine", "GenerateProof failed, err", err, "headerNumber", header.Number)
		return err
	}
	log.Debug("SHX profile", "generate block proof, blockNumber", header.Number, "proofHash", proof.Sign.Hash(), "time ", time.Now().UnixNano()/1000/1000)

	{
		// broadcast proof.
		msg := types.WorkProofMsg{
			Proof: *proof,
		}
		routEv := bc.RoutWorkProofEvent{
			ProofMsg: msg,
		}
		routEv.ProofMsg.Sign, err = self.engine.SignData(msg.Proof.Data())
		if err != nil {
			log.Debug("worker sign proof failed", "err", err)
			return err
		}
		self.mux.Post(routEv)
		log.Debug("worker proof goto wait confirm", "time ", time.Now().UnixNano()/1000/1000)

		handleLocalProof.Add(proof.Sign.Hash(), struct{}{})
		// wait confirm.
		self.unconfirm_mine.Insert(proof, work, consensus.MinerNumber/2+1-1)
	}
	go func(work *Work) {
		if block, err := self.engine.Finalize(self.chain, work.header, work.state, work.txs, work.states, work.receipts); err != nil {
			work.genCh <- err
		} else {
			log.Debug("worker after engine.Finalize", "time ", time.Now().UnixNano()/1000/1000)
			if result, err := self.engine.GenBlockWithSig(self.chain, block); err != nil {
				work.genCh <- err
			} else {
				log.Debug("worker after engine.GenBlockWithSig", "time ", time.Now().UnixNano()/1000/1000)
				work.Block = result
				work.genCh <- nil
			}
		}
	}(work)

	return nil
}

func (w *Work) WorkEnded(succeed bool) {
	txpool.GetTxPool().WorkEnded(w.id, w.header.Number.Uint64(), succeed)
}

func (self *worker) FinalMine(work *Work) error {
	// check work confirmed.
	var err error
	defer func() {
		if err != nil {
			go work.WorkEnded(false)
		}
	}()
	if work.confirmed {
		err = <-work.genCh
		if err == nil {
			result := work.Block
			self.history.Add(result.ProofHash(), struct{}{})

			if self.workPending.Add(work) {
				log.Info("Successfully sealed new block", "number -> ", result.Number(), "hash -> ", result.Hash(),
					"txs -> ", len(result.Transactions()))
				log.Debug("SHX profile worker", "sealed new block number ", result.Number(), "txs", len(result.Transactions()), "at time", time.Now().UnixNano()/1000/1000)
				return nil
			} else {
				err = errors.New("pending is rollback")
			}
		}
	} else {
		err = errors.New("block proof not confirmed")
	}
	return err
}
