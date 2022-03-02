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
package prometheus

import "C"
import (
	"bytes"
	"errors"
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/crypto"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/config"
	"github.com/shx-project/sphinx/consensus"
)

// Verify one header
func (c *Prometheus) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool, mode config.SyncMode) error {
	return c.verifyHeader(chain, header, nil, mode)
}

// Verify the headers
func (c *Prometheus) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool, mode config.SyncMode) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))

	go func() {
		for i, header := range headers {
			err := c.verifyHeader(chain, header, headers[:i], mode)

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

func (c *Prometheus) verifyHeader(chain consensus.ChainReader, header *types.Header, parents []*types.Header, mode config.SyncMode) error {
	if header.Number == nil {
		return consensus.ErrUnknownBlock
	}
	number := header.Number.Uint64()

	// Checkpoint blocks need to enforce zero beneficiary
	checkpoint := (number % consensus.NodeCheckpointInterval) == 0

	// Check that the extra-data contains both the vanity and signature
	if len(header.Extra) < consensus.ExtraVanity {
		return consensus.ErrMissingVanity
	}
	if len(header.Extra) < consensus.ExtraVanity+consensus.ExtraSeal {
		return consensus.ErrMissingSignature
	}
	// Ensure that the extra-data contains a signerHash list on checkpoint, but none otherwise
	signersBytes := len(header.Extra) - consensus.ExtraVanity - consensus.ExtraSeal
	if !checkpoint && signersBytes != 0 {
		return consensus.ErrExtraSigners
	}

	// All basic checks passed, verify cascading fields
	return c.verifyCascadingFields(chain, header, parents, mode)
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
func (c *Prometheus) verifyCascadingFields(chain consensus.ChainReader, header *types.Header, parents []*types.Header, mode config.SyncMode) error {
	// The genesis block is the always valid dead-end
	number := header.Number.Uint64()
	if number == 0 {
		return nil
	}
	// Ensure that the block's timestamp isn't too close to it's parent
	var parent *types.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}
	if parent == nil || parent.Number.Uint64() != number-1 || parent.Hash() != header.ParentHash {
		return consensus.ErrUnknownAncestor
	}
	if parent.Time.Uint64() > header.Time.Uint64() {
		return consensus.ErrInvalidTimestamp
	}

	// All basic checks passed, return
	return nil
}

func recoverpk2address(pub []byte) common.Address {
	var addr common.Address
	copy(addr[:], crypto.Keccak256(pub[1:])[12:])
	return addr
}

func (c *Prometheus) verifyProof(lHash common.Hash, address common.Address, proof *types.WorkProof) (common.Hash, error) {
	txroot := types.DeriveSha(proof.Txs)
	//proofRoot := types.DeriveSha(proof.States)
	//proofHash := c.MixHash(txroot,proofRoot)
	proofHash := c.MixHash(lHash, txroot)
	log.Debug("prometheus verify proof", "proofhash", proof.Sign.Hash(), "txroot", txroot, "localhash", lHash)

	if pub, err := crypto.Ecrecover(proofHash.Bytes(), proof.Sign); err == nil {
		addr := recoverpk2address(pub)
		if bytes.Compare(addr.Bytes(), address.Bytes()) != 0 {
			log.Debug("prometheus verify proof", "proof", proof.Sign.Hash(), "addr compare failed, recover addr", addr)
			return common.Hash{}, errors.New("invalid proof")
		} else {
			return proofHash, nil
		}
	} else {
		log.Debug("prometheus verify proof", "recover failed, err", err.Error())
		return common.Hash{}, errors.New("invalid proof")
	}
}
