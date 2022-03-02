package prometheus

import "github.com/shx-project/sphinx/blockchain/types"

// step 1. verify tx.
// step 2. dup tx detect.
// step 3. append tx to txpool.
// step 4. sign pooled txs Merkle-Tree root and broadcast to other peers.

func VerifyTx(tx *types.Transaction) bool {
	return true
}
func RoutineTxVarify(from chan *types.Transaction, to chan *types.Transaction, errCh chan interface{}) {
	for {
		select {
		case tx, ok := <-from:
			if !ok {
				// channel closed.
				return
			}
			if VerifyTx(tx) {
				to <- tx
			} else {
				errCh <- tx
			}
		}
	}
}

func DupDetect(tx *types.Transaction) bool {
	return false
}
func RoutineTxDupDetect(from chan *types.Transaction, to chan *types.Transaction, dupCh chan interface{}) {
	for {
		select {
		case tx, ok := <-from:
			if !ok {
				// channel closed.
				return
			}
			if DupDetect(tx) {
				dupCh <- tx
			} else {
				to <- tx
			}
		}
	}
}
