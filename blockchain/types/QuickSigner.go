package types

import (
	"errors"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/crypto"
	"github.com/shx-project/sphinx/common/log"
	"runtime"
)

var (
	ErrSignCheckFailed = errors.New("recover pubkey failed")
)

// result for recover pubkey
type RecoverPubkey struct {
	Tx	    *Transaction // recover tx's hash
	Hash   []byte
	Sig    []byte // signature
}

type TaskTh struct {
	isFull bool
	queue  chan *RecoverPubkey
}

type QSHandle struct {
	bInit     bool
	maxThNum  int                 // thread number for soft ecc recover.
	thPool    []*TaskTh           // task pool
	idx       int
}

var (
	qsHandle = &QSHandle{bInit: false, maxThNum: 2}
)

func QSGetInstance() *QSHandle {
	return qsHandle
}


func (qs *QSHandle) postToSoft(rs *RecoverPubkey) bool {

	for i := 0; i < qs.maxThNum; i++ {
		idx := (qs.idx + i) % qs.maxThNum
		select {
		case qs.thPool[idx].queue <- rs:
			qs.idx++
			return true
		default:
			log.Debug("qs", "thPool ", idx, "is full", len(qs.thPool[idx].queue))
		}
	}
	return false
}

// receive task and execute soft ecc-recover
func (qs *QSHandle) asyncSoftRecoverPubTask(queue chan *RecoverPubkey) {

	for {
		select {
		case rs, ok := <-queue:
			if !ok {
				return
			}
			pub, _ := crypto.Ecrecover(rs.Hash, rs.Sig)
			var addr = common.Address{}
			copy(addr[:], crypto.Keccak256(pub[1:])[12:])
		}
	}
}

func (qs *QSHandle) Init() error {
	if qs.bInit {
		return nil
	}

	// use 1/4 cpu
	if runtime.NumCPU() > qs.maxThNum {
		qs.maxThNum = runtime.NumCPU() - 1
	}

	qs.thPool = make([]*TaskTh, qs.maxThNum)

	for i := 0; i < qs.maxThNum; i++ {
		qs.thPool[i] = &TaskTh{isFull: false, queue: make(chan *RecoverPubkey, 1000000)}

		go qs.asyncSoftRecoverPubTask(qs.thPool[i].queue)
	}

	return nil
}

func (qs *QSHandle) Release() error {
	for i := 0; i < qs.maxThNum; i++ {
		close(qs.thPool[i].queue)
	}
	return nil
}

func softRecoverPubkey(hash []byte, r []byte, s []byte, v byte) ([]byte, error) {
	var (
		result = make([]byte, 65)
		sig    = make([]byte, 65)
	)
	copy(sig[32-len(r):32], r)
	copy(sig[64-len(s):64], s)
	sig[64] = v
	pub, err := crypto.Ecrecover(hash[:], sig)
	if err != nil {
		return nil, ErrSignCheckFailed
	}
	copy(result[:], pub[0:])
	return result, nil
}

func (qs *QSHandle) ASyncValidateSign(tx *Transaction, hash []byte, r []byte, s []byte, v byte) error {
	rs := RecoverPubkey{Tx:tx, Hash: make([]byte, 32), Sig: make([]byte, 65)}
	copy(rs.Hash, hash)
	copy(rs.Sig[32-len(r):32], r)
	copy(rs.Sig[64-len(s):64], s)
	rs.Sig[64] = v
	qs.postToSoft(&rs)

	return nil
}

func (qs *QSHandle) ValidateSign(hash []byte, r []byte, s []byte, v byte) ([]byte, error) {
	return softRecoverPubkey(hash, r, s, v)
}

func init() {
	QSGetInstance().Init()
}
