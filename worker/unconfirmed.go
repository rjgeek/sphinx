// Copyright 2016 The go-hbp Authors
// This file is part of the go-hbp library.
//
// The go-hbp library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-hbp library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-hbp library. If not, see <http://www.gnu.org/licenses/>.

package worker

import (
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/log"
	"gopkg.in/fatih/set.v0"
	"sync"
	"time"
)

type proofInfo struct {
	threshold int
	work  *Work
	confirmed *set.Set
	confirmedUnpass *set.Set
	time  int64
}

type unconfirmedProofs struct {
	checking 	bool
	// proof --> proofInfo
	proofs 		sync.Map // map[hash(signature)]proofInfo record proof's info.
	confirmedCh chan *Work
	stopCh      chan struct{}
}

func newUnconfirmedProofs(confirmedCh chan *Work) *unconfirmedProofs{
	return &unconfirmedProofs{
		confirmedCh:confirmedCh,
		stopCh:make(chan struct{}),
		checking:false,
	}
}

func (u *unconfirmedProofs) Insert(proof *types.WorkProof, work *Work, threshold int) error {
	sigHash := proof.Sign.Hash()
	if _, ok := u.proofs.Load(sigHash); !ok {
		info := &proofInfo{threshold:threshold, work:work, confirmed:set.New(), confirmedUnpass:set.New(), time:time.Now().Unix()}
		u.proofs.Store(sigHash, info)
		return nil
	}
	return nil
}

func (u *unconfirmedProofs) Confirm(addr common.Address, confirm *types.ProofConfirm) error {
	sigHash := confirm.Signature.Hash()
	if v, ok := u.proofs.Load(sigHash); ok {
		info := v.(*proofInfo)
		if confirm.Confirm == true {
			log.Debug("worker confirm , add confirm");
			info.confirmed.Add(addr)
		} else {
			log.Debug("worker confirm unpass, add confirm");
			info.confirmedUnpass.Add(addr)
		}
		if info.confirmed.Size() >= info.threshold || info.confirmedUnpass.Size() >= info.threshold {
			if info.confirmedUnpass.Size() >= info.threshold {
				log.Info("workconfirm, confirmunpass enough")
				info.work.confirmed = false
			} else {
				log.Debug("worker confirm , confirm enough")
				info.work.confirmed = true
			}
			// send to worker.
			go func(){
				defer func() {
					if err := recover();err != nil {
						log.Debug("error on confirmedCh","err", err)
					}
				}()
				u.confirmedCh <- info.work
			}()
			u.proofs.Delete(sigHash)
		}
	}
	log.Debug("exit confirm function")
	return nil
}

func (u *unconfirmedProofs) Stop() {
	u.stopCh <- struct{}{}
}

func (u *unconfirmedProofs) CheckTimeout() {
	u.checking = true
	defer func() { u.checking = false } ()

	u.proofs.Range(func(k, v interface{}) bool {
		info := v.(*proofInfo)
		now := time.Now().Unix()
		time.Now().Sub(info.work.createdAt)
		if now - info.time > waitConfirmTimeout {
			// unconfirmed proof, drop work.
			go func(){u.confirmedCh <- info.work} ()
			u.proofs.Delete(k)
		}
		return true
	})
}


func (u *unconfirmedProofs) RoutineLoop () {
	evict := time.NewTicker(5 * time.Second)
	defer evict.Stop()
	for {
		select {
		case <-evict.C:
			if !u.checking {
				go u.CheckTimeout()
			}
		case <-u.stopCh:
			return
		}
	}
}
