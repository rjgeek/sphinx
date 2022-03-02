package worker

import (
	"errors"
	"github.com/hashicorp/golang-lru"
	"github.com/shx-project/sphinx/blockchain"
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/txpool"
	"gopkg.in/fatih/set.v0"
	"math/big"
	"sync/atomic"
	"time"
)

var handleLocalProof, _ = lru.New(100000)
var queryCache, _ = lru.New(100000)

var cap_block_count uint64 = 5

func (self *worker) updateTxConfirm() {
	self.txMu.Lock()
	defer self.txMu.Unlock()
	self.updating = true
	batch := self.chainDb.NewBatch()
	cnt := 0
	for hash, _ := range self.txConfirmPool {
		if receipts, blockHash, blockNumber, err := bc.GetBlockReceiptsByTx(self.chainDb, hash); err == nil {
			for _, receipt := range receipts {
				if confirm, ok := self.txConfirmPool[receipt.TxHash]; ok {
					cnt++
					receipt.ConfirmCount += confirm
					delete(self.txConfirmPool, receipt.TxHash)
				}
			}
			bc.WriteBlockReceipts(batch, blockHash, blockNumber, receipts)
		}
		if cnt > 10000 {
			break
		}
	}
	//log.Debug("worker updateTxConfirm, before batch.write")
	if cnt > 0 {
		batch.Write()
	}
	//log.Debug("worker updateTxConfirm, after batch.write", "cnt ",cnt)
	self.updating = false
}

func (self *worker) queryRemoteState(miner common.Address, number *big.Int, timeout int) error {
	var err error
	// request BatchProofsData from peer
	msg := types.QueryStateMsg{
		Qs: types.QueryState{Miner: miner, Number: *number},
	}
	msg.Sign, err = self.engine.SignData(msg.Qs.Data())
	if err != nil {
		log.Debug("worker query state sign failed", "err", err)
		return err
	}
	queryCache.Add(miner, msg)

	ev := bc.RoutQueryStateEvent{msg}
	self.mux.Post(ev)

	var peerlock chan struct{}
	self.mu.Lock()
	if l, ok := self.peerLockMap[miner]; !ok {
		peerlock = make(chan struct{})
		self.peerLockMap[miner] = peerlock
	} else {
		peerlock = l
	}
	self.mu.Unlock()
	log.Debug("getBatchProofs before <-peerlock", "peer is ", miner)
	timer := time.NewTimer(time.Duration(timeout) * time.Second)
	defer timer.Stop()
	select {
	case <-peerlock:
		log.Debug("getBatchProofs after <-peerlock", "peer is ", miner)
		return nil
	case <-timer.C:
		log.Debug("getBatchProofs timeout ", "peer is ", miner)
		self.mu.Lock()
		self.peerLockMap[miner] = nil
		self.mu.Unlock()
		return errors.New("timeout")
	}
}

func (self *worker) dealProofEvent(ev *types.WorkProofMsg, sender common.Address) {
	// 1. receive proof
	// 2. verify proof
	if sender == self.coinbase {
		// ignore self proof.
		return
	}
	var err error
	pastLocalRoot := set.New(self.history.Keys()...)

	if atomic.LoadInt32(&self.mining) == 1 {
		if self.current != nil && self.current.header != nil {
			pastLocalRoot.Add(self.current.header.ProofHash)
		}
	}

	confirm := types.ConfirmMsg{
		Confirm: types.ProofConfirm{Signature: ev.Proof.Sign, Confirm: false},
	}
	routEv := bc.RoutConfirmEvent{confirm}

	if !self.engine.VerifyState(self.coinbase, pastLocalRoot, &ev.Proof) {
		if routEv.ConfirmMsg.Sign, err = self.engine.SignData(routEv.ConfirmMsg.Confirm.Data()); err != nil {
			log.Debug("worker deal proof event, sign confirm failed", "err", err)
			return
		}
		self.mux.Post(routEv)
		return
	}
	peerProofState := bc.GetPeerProof(self.chainDb, sender)

	evnum := ev.Proof.Number.Uint64()
	// if local have no peer proof state or this is a re-mined block with last n count, we can resync remote state.
	if peerProofState == nil {
		if evnum == 1 {
			// the first block from peer, lastHash is genesis.ProofHash
			genesis := self.chain.GetHeaderByNumber(0)
			peerProofState = &types.ProofState{Addr: sender, Num: 0, Root: genesis.ProofHash}
		} else {
			// request BatchProofsData from peer
			log.Debug("worker goto queryRemoteState", "from ", sender, "number", evnum-1)

			if err := self.queryRemoteState(sender, big.NewInt(0).Sub(&ev.Proof.Number, big.NewInt(1)), 30); err == nil {
				peerProofState = bc.GetPeerProof(self.chainDb, sender)
			} else {
				if routEv.ConfirmMsg.Sign, err = self.engine.SignData(routEv.ConfirmMsg.Confirm.Data()); err != nil {
					log.Debug("worker deal proof event, sign confirm failed", "err", err)
					return
				}
				self.mux.Post(routEv)
				return
			}
		}
	}

	for {
		// loop to request missed proof and re-verify again.
		if newroot, err := self.engine.VerifyProof(sender, peerProofState.Root, &ev.Proof); err != nil {
			snum := peerProofState.Num + 1
			if snum == evnum {
				log.Debug("worker verify proof proofhash failed")
				if routEv.ConfirmMsg.Sign, err = self.engine.SignData(routEv.ConfirmMsg.Confirm.Data()); err != nil {
					log.Debug("worker deal proof event, sign confirm failed", "err", err)
					return
				}
				self.mux.Post(routEv)
				return
			} else {

				log.Debug("worker verify proof", "query remote state from", sender, "number", evnum-1)
				// request missed proof.
				err := self.queryRemoteState(sender, big.NewInt(int64(evnum-1)), 30)
				if err != nil {
					break
				}
				peerProofState = bc.GetPeerProof(self.chainDb, sender)
			}
		} else {
			// update peer's proof in local.
			updateProof := types.ProofState{Addr: sender, Num: ev.Proof.Number.Uint64(), Root: newroot}
			bc.WritePeerProof(self.chainDb, sender, updateProof)

			routEv.ConfirmMsg.Confirm.Confirm = true
			break
		}
	}

	if routEv.ConfirmMsg.Sign, err = self.engine.SignData(routEv.ConfirmMsg.Confirm.Data()); err != nil {
		log.Debug("worker deal proof event, sign confirm failed", "err", err)
		return
	}
	log.Debug("worker post proof confirm ", "confirm is ", routEv.ConfirmMsg.Confirm.Confirm)
	self.mux.Post(routEv)

	if routEv.ConfirmMsg.Confirm.Confirm == false {
		return
	}

	// add tx to txpool.
	go txpool.GetTxPool().AddTxs(ev.Proof.Txs)
	// 3. update tx info (tx's signed count)
	self.txMu.Lock()
	for _, tx := range ev.Proof.Txs {
		// add to unconfirmed tx.
		if v, ok := self.txConfirmPool[tx.Hash()]; ok {
			v += 1
			self.txConfirmPool[tx.Hash()] = v
			//log.Debug("worker update tx map", "hash", tx.Hash(), "count", v)
		} else {
			self.txConfirmPool[tx.Hash()] = 1
			//log.Debug("worker update tx map", "new hash", tx.Hash(), "count", 1)
		}
	}
	self.txMu.Unlock()
}

func (self *worker) dealConfirm(ev *types.ConfirmMsg, sender common.Address) {
	// 1. receive proof response
	// 2. calc response count
	// 3. if count > peers/2 , final mined.

	if _, ok := handleLocalProof.Get(ev.Confirm.Signature.Hash()); ok {
		log.Debug("SHX profile", "get confirm for Proof ", ev.Confirm.Signature.Hash(), "from minenode", sender, "at time ", time.Now().UnixNano()/1000/1000)
		self.unconfirm_mine.Confirm(sender, &ev.Confirm)
	}
}

func (self *worker) dealQueryState(ev *types.QueryStateMsg, sender common.Address) {
	var err error
	var root common.Hash
	if ev.Qs.Miner != self.coinbase {
		pstate := bc.GetPeerProof(self.chainDb, ev.Qs.Miner)
		if pstate != nil && pstate.Num == ev.Qs.Number.Uint64() {
			root = pstate.Root
		} else {
			return
		}
	} else {
		h := self.chain.GetHeaderByNumber(ev.Qs.Number.Uint64())
		if h == nil {
			log.Debug("worker deal query state, get header is nil", "number", ev.Qs.Number)
			return
		} else {
			log.Debug("dealQueryState ", "from", sender, "num", ev.Qs.Number.Uint64())
			root = h.ProofHash
		}
	}
	msg := types.ResponseStateMsg{
		Rs: types.ResponseState{
			Querier: sender,
			Number:  ev.Qs.Number,
			Root:    root,
		},
	}

	routEv := bc.RoutResponseStateEvent{msg}
	routEv.Rs.Sign, err = self.engine.SignData(routEv.Rs.Rs.Data())
	if err != nil {
		return
	}
	self.mux.Post(routEv)
}

func (self *worker) dealResponseState(ev *types.ResponseStateMsg, sender common.Address) {
	if ev.Rs.Querier != self.coinbase {
		// the message is not for me, just broadcast to other.
		return
	}
	q, exist := queryCache.Get(sender)
	if !exist {
		return
	}
	qs := q.(types.QueryStateMsg)
	if ev.Rs.Number.Uint64() != qs.Qs.Number.Uint64() {
		log.Debug("worker got number unmatched response state", "from ", sender, "number", ev.Rs.Number)
		return
	}

	//oldState := bc.GetPeerProof(self.chainDb, sender)
	//if oldState != nil && oldState.Num > ev.Rs.Number.Uint64() {
	//	log.Debug("worker deal ResponseState got an lower state","response number ", ev.Rs.Number, "local number", oldState.Num)
	//	return
	//}
	//
	peerstate := types.ProofState{
		Addr: sender,
		Num:  ev.Rs.Number.Uint64(),
		Root: ev.Rs.Root,
	}

	bc.WritePeerProof(self.chainDb, sender, peerstate)
	log.Debug("PeerProof update", "addr", sender, "to", peerstate.Num)

	self.mu.Lock()
	ch := self.peerLockMap[sender]
	self.mu.Unlock()
	if ch != nil {
		ch <- struct{}{}
	}
}
