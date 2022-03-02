package types

import (
	"bytes"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/crypto/sha3"
	"github.com/shx-project/sphinx/common/merkletree"
	"github.com/shx-project/sphinx/common/rlp"
	"math/big"
)

type ProofState struct {
	Addr 	common.Address
	Root 	common.Hash
	Num     uint64
}

func (p ProofState)CalculateHash()([]byte, error){
	ret := p.Root.Bytes()
	return ret,nil
}

func (p ProofState)Equals(other merkletree.Content)(bool, error){
	op := other.(ProofState)
	if p.Root == op.Root && p.Addr == op.Addr {
		return true,nil
	}
	return false, nil
}

type ProofStates []*ProofState

// Len returns the length of s
func (s ProofStates) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s
func (s ProofStates) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// GetRlp implements Rlpable and returns the i'th element of s in rlp
func (s ProofStates) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}

func (s ProofStates)GetMerkleContent(i int) merkletree.Content{
	return s[i]
}


type ProofSignature []byte
func (p ProofSignature)Hash() common.Hash{
	h := common.Hash{}
	hash := sha3.Sum256(p)
	h.SetBytes(hash[:])
	return h
}


type WorkProofMsg struct {
	Proof WorkProof
	Sign []byte
}
func (wpm WorkProofMsg)Hash() common.Hash {
	s := sha3.New256()
	s.Write(wpm.Proof.Data())
	s.Write(wpm.Sign)
	hash := common.Hash{}
	hash.SetBytes(s.Sum(nil))

	return hash
}

type ConfirmMsg struct {
	Confirm ProofConfirm
	Sign []byte
}
func (cfm ConfirmMsg)Hash() common.Hash {
	s := sha3.New256()
	s.Write(cfm.Confirm.Data())
	s.Write(cfm.Sign)
	hash := common.Hash{}
	hash.SetBytes(s.Sum(nil))

	return hash
}



type QueryStateMsg struct {
	Qs QueryState
	Sign []byte
}

func (qsm QueryStateMsg)Hash() common.Hash {
	s := sha3.New256()
	s.Write(qsm.Qs.Data())
	s.Write(qsm.Sign)
	hash := common.Hash{}
	hash.SetBytes(s.Sum(nil))
	return hash
}



type ResponseStateMsg struct {
	Rs ResponseState
	Sign []byte
}

func (rsm ResponseStateMsg)Hash() common.Hash {
	s := sha3.New256()
	s.Write(rsm.Rs.Data())
	s.Write(rsm.Sign)
	hash := common.Hash{}
	hash.SetBytes(s.Sum(nil))
	return hash
}

type WorkProof struct {
	Number 	  big.Int
	Sign 	  ProofSignature
	Txs       Transactions
	States 	  ProofStates
}

// return data to sign a signature.
func (wp WorkProof)Data() []byte {
	buf := bytes.NewBuffer([]byte{})
	buf.Write(wp.Number.Bytes())
	buf.Write(wp.Sign)
	return buf.Bytes()
}

type ProofConfirm struct {
	Signature ProofSignature
	Confirm   bool
}

func (pc ProofConfirm)Data() []byte {
	buf := bytes.NewBuffer([]byte{})
	buf.Write(pc.Signature)
	if pc.Confirm {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
	return buf.Bytes()
}

type QueryState struct {
	Miner common.Address
	Number big.Int
}

func (qs QueryState)Data() []byte {
	buf := bytes.NewBuffer([]byte{})
	buf.Write(qs.Number.Bytes())
	buf.Write(qs.Miner.Bytes())
	return buf.Bytes()
}

type ResponseState struct {
	Number big.Int
	Root common.Hash
	Querier common.Address
}

func (rs ResponseState )Data() []byte {
	buf := bytes.NewBuffer([]byte{})
	buf.Write(rs.Number.Bytes())
	buf.Write(rs.Root.Bytes())
	buf.Write(rs.Querier.Bytes())
	return buf.Bytes()
}
