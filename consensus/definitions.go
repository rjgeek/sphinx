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

package consensus

import (
	"errors"

	"github.com/hashicorp/golang-lru"
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/crypto"
	"github.com/shx-project/sphinx/common/crypto/sha3"
	"github.com/shx-project/sphinx/common/rlp"
)

const NodeCheckpointInterval = 200

var (
	IgnoreRetErr = false //ignore finalize return err
)

var (
	// ErrUnknownAncestor is returned when validating a block requires an ancestor
	// that is unknown.
	ErrUnknownAncestor = errors.New("unknown ancestor")

	// ErrFutureBlock is returned when a block's timestamp is in the future according
	// to the current node.
	ErrFutureBlock = errors.New("block in the future")

	// ErrInvalidNumber is returned if a block's number doesn't equal it's parent's
	// plus one.
	ErrInvalidNumber = errors.New("invalid block number")

	// extra-data
	ErrMissingVanity = errors.New("extra-data 32 byte vanity prefix missing")

	ErrMissingSignature = errors.New("extra-data 65 byte suffix signature missing")

	ErrExtraSigners = errors.New("non-checkpoint block contains extra signer list")

	ErrInvalidMixDigest = errors.New("non-zero mix digest")

	ErrInvalidUncleHash = errors.New("non empty uncle hash")

	ErrInvalidTimestamp = errors.New("invalid timestamp")

	ErrUnauthorized = errors.New("unauthorized")

	ErrWaitTransactions = errors.New("waiting for transactions")

	ErrUnknownBlock = errors.New("unknown block")

	ErrInvalidCheckpointBeneficiary = errors.New("beneficiary in checkpoint block non-zero")

	ErrInvalidVote = errors.New("vote nonce not 0x00..0 or 0xff..f")

	// vote nonce in checkpoint block non-zero
	ErrInvalidCheckpointVote = errors.New("vote nonce in checkpoint block non-zero")
	// reject block but do not drop peer
	ErrInvalidblockbutnodrop = errors.New("reject block but do not drop peer")
	// bad param
	ErrBadParam    = errors.New("input bad param")
	Errnilparam    = errors.New("input param is nil")
	ErrNoLastBlock = errors.New("No Last Block when verify during the fullsync")
)

var (
	Zeroaddr = common.HexToAddress("0x0000000000000000000000000000000000000000")

	ExtraVanity = 32 // Fixed number of extra-data prefix bytes reserved for signerHash vanity
	ExtraSeal   = 65 // Fixed number of extra-data suffix bytes reserved for signerHash seal
)

const (
	MinerNumber = 8
)

// get current signer
func Ecrecover(header *types.Header, sigcache *lru.ARCCache) (common.Address, error) {

	hash := header.Hash()
	if address, known := sigcache.Get(hash); known {
		return address.(common.Address), nil
	}

	if len(header.Extra) < ExtraSeal {
		return common.Address{}, ErrMissingSignature
	}
	signature := header.Extra[len(header.Extra)-ExtraSeal:]

	// recover the public key
	pubkey, err := crypto.Ecrecover(SigHash(header).Bytes(), signature)
	if err != nil {
		return common.Address{}, err
	}
	var signer common.Address
	copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])

	sigcache.Add(hash, signer)
	return signer, nil
}

func SigHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewKeccak256()

	rlp.Encode(hasher, []interface{}{
		header.ParentHash,
		header.Root,
		header.TxHash,
		header.ReceiptHash,
		header.Difficulty,
		header.Number,
		header.Time,
		header.Extra[:len(header.Extra)-65],
	})
	hasher.Sum(hash[:0])
	return hash
}
