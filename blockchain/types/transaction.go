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

package types

import (
	"bytes"
	"container/heap"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"encoding/json"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/crypto/sha3"
	"github.com/shx-project/sphinx/common/hexutil"
	"github.com/shx-project/sphinx/common/merkletree"
	"github.com/shx-project/sphinx/common/rlp"
)

//go:generate gencodec -type txdata -field-override txdataMarshaling -out gen_tx_json.go

var (
	ErrInvalidSig = errors.New("invalid transaction v, r, s values")
	errNoSigner   = errors.New("missing signing methods")
)

type Transaction struct {
	data txdata
	// caches
	Forward bool
	hash atomic.Value
	size atomic.Value
}

type txdata struct {
	Payload []byte `json:"input"    gencodec:"required"`
	// This is only used when marshaling to JSON.
	Hash *common.Hash `json:"hash" rlp:"-"`
}

type txdataMarshaling struct {
	Payload hexutil.Bytes
}

func NewTransaction(data []byte) *Transaction {
	return newTransaction(data)
}

func newTransaction(data []byte) *Transaction {
	if len(data) > 0 {
		data = common.CopyBytes(data)
	}
	d := txdata{
		Payload: data,
	}

	return &Transaction{data: d}
}

// DecodeRLP implements rlp.Encoder
func (tx *Transaction) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &tx.data)
}

// DecodeRLP implements rlp.Decoder
func (tx *Transaction) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()
	err := s.Decode(&tx.data)
	if err == nil {
		tx.size.Store(common.StorageSize(rlp.ListSize(size)))
	}

	return err
}

func (t txdata) MarshalJSON() ([]byte, error) {
	type txdata struct {
		Payload hexutil.Bytes `json:"input"    gencodec:"required"`
		Hash    *common.Hash  `json:"hash" rlp:"-"`
	}
	var enc txdata
	enc.Payload = t.Payload
	enc.Hash = t.Hash
	return json.Marshal(&enc)
}

func (t *txdata) UnmarshalJSON(input []byte) error {
	type txdata struct {
		Payload hexutil.Bytes `json:"input"    gencodec:"required"`
		Hash    *common.Hash  `json:"hash" rlp:"-"`
	}
	var dec txdata
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	t.Payload = dec.Payload
	if dec.Hash != nil {
		t.Hash = dec.Hash
	}
	return nil
}

func (tx *Transaction) MarshalJSON() ([]byte, error) {
	hash := tx.Hash()
	data := tx.data
	data.Hash = &hash
	return data.MarshalJSON()
}

// UnmarshalJSON decodes the web3 RPC transaction format.
func (tx *Transaction) UnmarshalJSON(input []byte) error {
	var dec txdata
	if err := dec.UnmarshalJSON(input); err != nil {
		return err
	}
	*tx = Transaction{data: dec}
	return nil
}

func (tx *Transaction) Data() []byte     { return common.CopyBytes(tx.data.Payload) }
func (tx *Transaction) CheckNonce() bool { return true }

func (tx *Transaction) SetForward(forward bool)     { tx.Forward = forward }
func (tx *Transaction) IsForward() bool             { return tx.Forward }

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

// Hash hashes the RLP encoding of tx.
// It uniquely identifies the transaction.
func (tx *Transaction) Hash() common.Hash {
	if hash := tx.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	temp := tx.IsForward()
	tx.SetForward(false)
	v := rlpHash(tx)
	tx.SetForward(temp)
	tx.hash.Store(v)
	return v
}

type writeCounter common.StorageSize

func (c *writeCounter) Write(b []byte) (int, error) {
	*c += writeCounter(len(b))
	return len(b), nil
}

func (tx *Transaction) Size() common.StorageSize {
	if size := tx.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := writeCounter(0)
	rlp.Encode(&c, &tx.data)
	tx.size.Store(common.StorageSize(c))
	return common.StorageSize(c)
}

func (tx *Transaction) String() string {
	enc, _ := rlp.EncodeToBytes(&tx.data)
	return fmt.Sprintf(`
	TX(%x)
	Data:     0x%x
	Hex:      %x
`,
		tx.Hash(),
		tx.data.Payload,
		enc,
	)
}

func (tx *Transaction)CalculateHash()([]byte, error){
	hash := tx.Hash()
	return hash[:],nil
}

func (tx *Transaction)Equals(other merkletree.Content)(bool,error) {
	ohash := other.(*Transaction).Hash()
	if bytes.Compare(ohash.Bytes(), tx.Hash().Bytes()) == 0 {
		return true,nil
	}
	return false, nil
}

// Transaction slice type for basic sorting.
type Transactions []*Transaction

// Len returns the length of s
func (s Transactions) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s
func (s Transactions) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// GetRlp implements Rlpable and returns the i'th element of s in rlp
func (s Transactions) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}

func (s Transactions)GetMerkleContent(i int) merkletree.Content{
	return s[i]
}


// Returns a new set t which is the difference between a to b
func TxDifference(a, b Transactions) (keep Transactions) {
	keep = make(Transactions, 0, len(a))

	remove := make(map[common.Hash]struct{})
	for _, tx := range b {
		remove[tx.Hash()] = struct{}{}
	}

	for _, tx := range a {
		if _, ok := remove[tx.Hash()]; !ok {
			keep = append(keep, tx)
		}
	}

	return keep
}

type TxByPayload Transactions

func (s TxByPayload) Len() int           { return len(s) }
func (s TxByPayload) Less(i, j int) bool { return len(s[i].data.Payload) < len(s[j].data.Payload) }
func (s TxByPayload) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s *TxByPayload) Push(x interface{}) {
	*s = append(*s, x.(*Transaction))
}

func (s *TxByPayload) Pop() interface{} {
	old := *s
	n := len(old)
	x := old[n-1]
	*s = old[0 : n-1]
	return x
}

type TransactionsByPayload struct {
	heads TxByPayload // Next transaction for each unique account (price heap)
}

// NewTransactionsByPayload creates a transaction set that can retrieve
// payload length sorted transactions.
//
// Note, the input map is reowned so the caller should not interact any more with
// if after providing it to the constructor.
func NewTransactionsByPayload(txs Transactions) *TransactionsByPayload {
	// Initialize a price based heap with the head transactions
	heads := make(TxByPayload, 0, len(txs))
	for _, tx := range txs {
		heads = append(heads, tx)
	}
	heap.Init(&heads)

	// Assemble and return the transaction set
	return &TransactionsByPayload{
		heads: heads,
	}
}

// Peek returns the next transaction by price.
func (t *TransactionsByPayload) Peek() *Transaction {
	if len(t.heads) == 0 {
		return nil
	}
	return t.heads[0]
}

// Shift replaces the current best head with the next one from the same account.
func (t *TransactionsByPayload) Shift() {
	heap.Pop(&t.heads)
}

// Pop removes the best transaction, *not* replacing it with the next one from
// the same account. This should be used when a transaction cannot be executed
// and hence all subsequent ones should be discarded from the same account.
func (t *TransactionsByPayload) Pop() {
	heap.Pop(&t.heads)
}
