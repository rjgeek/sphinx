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
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/crypto"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/config"
	"math/big"
)

var (
	ErrInvalidChainId    = errors.New("invalid chain id for signer")
	ErrInvalidAsynsinger = errors.New("invalid chain id  Asyn Send OK for signer")
)

// sigCache is used to cache the derived sender and contains
// the signer used to derive it.
type sigCache struct {
	from   common.Address
}

// MakeSigner returns a Signer based on the given chain config and block number.
func MakeSigner(config *config.ChainConfig) Signer {
	return NewQSSigner(config.ChainId)
}

// SignTx signs the transaction using the given signer and private key
func SignTx(tx *Transaction, s Signer, prv *ecdsa.PrivateKey) (*Transaction, error) {
	return tx, nil
}

// Signer encapsulates transaction signature handling. Note that this interface is not a
// stable API and may change at any time to accommodate new protocol rules.
type Signer interface {
	// SignatureValues returns the raw R, S, V values corresponding to the
	// given signature.
	SignatureValues(tx *Transaction, sig []byte) (r, s, v *big.Int, err error)
	// Hash returns the hash to be signed.
	Hash(tx *Transaction) common.Hash
	// Equal returns true if the given signer is the same as the receiver.
	Equal(Signer) bool
}

// EIP155Transaction implements Signer using the EIP155 rules.
type QSSigner struct {
	chainId, chainIdMul *big.Int
}

func NewQSSigner(chainId *big.Int) QSSigner {
	if chainId == nil {
		chainId = new(big.Int)
	}

	return QSSigner{
		chainId:    chainId,
		chainIdMul: new(big.Int).Mul(chainId, big.NewInt(2)),
	}
}

func (s QSSigner) Equal(s2 Signer) bool {
	eip155, ok := s2.(QSSigner)

	return ok && (eip155.chainId.Cmp(s.chainId) == 0)
}

var big8 = big.NewInt(8)


// WithSignature returns a new transaction with the given signature. This signature
// needs to be in the [R || S || V] format where V is 0 or 1.
func (s QSSigner) SignatureValues(tx *Transaction, sig []byte) (R, S, V *big.Int, err error) {
	if len(sig) != 65 {
		panic(fmt.Sprintf("wrong size for signature: got %d, want 65", len(sig)))
	}
	R = new(big.Int).SetBytes(sig[:32])
	S = new(big.Int).SetBytes(sig[32:64])
	V = new(big.Int).SetBytes([]byte{sig[64] + 27})
	if s.chainId.Sign() != 0 {
		V = big.NewInt(int64(sig[64] + 35))
		V.Add(V, s.chainIdMul)
	}
	return R, S, V, nil
}

// Hash returns the hash to be signed by the sender.
// It does not uniquely identify the transaction.
func (s QSSigner) Hash(tx *Transaction) common.Hash {
	return rlpHash([]interface{}{
		tx.data.Payload,
		s.chainId, uint(0), uint(0),
	})
}

func recoverPlain(sighash common.Hash, R, S, Vb *big.Int) (common.Address, error) {
	if Vb.BitLen() > 8 {
		return common.Address{}, ErrInvalidSig
	}
	V := byte(Vb.Uint64() - 27)
	if !crypto.ValidateSignatureValues(V, R, S, true) {
		return common.Address{}, ErrInvalidSig
	}

	// encode the snature in uncompressed format
	r, s := R.Bytes(), S.Bytes()

	pub, err := QSGetInstance().ValidateSign(sighash.Bytes(), r, s, V)
	if err != nil {
		log.Trace("qs validatesign error")
		return common.Address{}, err
	}

	if len(pub) == 0 || pub[0] != 4 {
		return common.Address{}, errors.New("invalid public key")
	}
	var addr common.Address
	copy(addr[:], crypto.Keccak256(pub[1:])[12:])
	return addr, nil
}

func ASynrecoverPlain(tx *Transaction, sighash common.Hash, R, S, Vb *big.Int) (common.Address, error) {

	if Vb.BitLen() > 8 {
		log.Error("ASynrecoverPlain Vb.BitLen() > 8")
		return common.Address{}, ErrInvalidSig
	}
	V := byte(Vb.Uint64() - 27)
	if !crypto.ValidateSignatureValues(V, R, S, true) {
		log.Error("ASynrecoverPlain !crypto.ValidateSignatureValues")
		return common.Address{}, ErrInvalidSig
	}
	r, s := R.Bytes(), S.Bytes()

	err := QSGetInstance().ASyncValidateSign(tx, sighash.Bytes(), r, s, V)
	if err != nil {
		log.Trace("qs validatesign error")
		return common.Address{}, err
	}
	return common.Address{}, ErrInvalidAsynsinger
}

// deriveChainId derives the chain id from the given v parameter
func deriveChainId(v *big.Int) *big.Int {
	if v.BitLen() <= 64 {
		v := v.Uint64()
		if v == 27 || v == 28 {
			return new(big.Int)
		}
		return new(big.Int).SetUint64((v - 35) / 2)
	}
	v = new(big.Int).Sub(v, big.NewInt(35))
	return v.Div(v, big.NewInt(2))
}
