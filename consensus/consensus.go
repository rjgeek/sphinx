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

// Package consensus implements different Shx consensus engines.
package consensus

import (
	"github.com/shx-project/sphinx/blockchain/state"
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common"
	"gopkg.in/fatih/set.v0"

	//"github.com/shx-project/sphinx/common/constant"
	"github.com/shx-project/sphinx/config"
	"github.com/shx-project/sphinx/network/rpc"
)

// ChainReader defines a small collection of methods needed to access the local
// blockchain during header and/or uncle verification.
type ChainReader interface {
	// Config retrieves the blockchain's chain configuration.
	Config() *config.ChainConfig

	// CurrentHeader retrieves the current header from the local chain.
	CurrentHeader() *types.Header

	// GetHeader retrieves a block header from the database by hash and number.
	GetHeader(hash common.Hash, number uint64) *types.Header

	// GetHeaderByNumber retrieves a block header from the database by number.
	GetHeaderByNumber(number uint64) *types.Header

	// GetHeaderByash retrieves a block header from the database by its hash.
	GetHeaderByHash(hash common.Hash) *types.Header

	// GetBlock retrieves a block from the database by hash and number.
	GetBlock(hash common.Hash, number uint64) *types.Block

	StateAt(root common.Hash) (*state.StateDB, error)
}

// Engine is an algorithm agnostic consensus engine.
type Engine interface {
	// generate
	PrepareBlockHeader(chain ChainReader, header *types.Header, state *state.StateDB) error

	// GerateProof return a workproof
	GenerateProof(chain ChainReader, header *types.Header, parent *types.Header, txs types.Transactions, proofs types.ProofStates) (*types.WorkProof, error)
	SignData(data []byte) ([]byte, error)
	RecoverSender(data []byte, signature []byte) (common.Address, error)

	// VerifyProof check the proof from peer is correct, and return new hash.
	VerifyProof(addr common.Address, lastHash common.Hash, proof *types.WorkProof) (common.Hash, error)
	VerifyState(coinbase common.Address, history *set.Set, proof *types.WorkProof) bool
	VerifyProofQuick(lasthash common.Hash, txroot common.Hash, newHash common.Hash) error

	// Finalize runs any post-transaction state modifications
	// and assembles the final block.
	// Note: The block header and state database might be updated to reflect any
	// consensus rules that happen at finalization.
	Finalize(chain ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
		proofs []*types.ProofState, receipts []*types.Receipt) (*types.Block, error)

	// Seal generates a new block for the given input block with the local miner's
	// seal place on top.
	GenBlockWithSig(chain ChainReader, block *types.Block) (*types.Block, error)

	// Author retrieves the Shx address of the account that minted the given
	// block, which may be different from the header's coinbase if a consensus
	// engine is based on signatures.
	Author(header *types.Header) (common.Address, error)

	// VerifyHeader checks whether a header conforms to the consensus rules of a
	// given engine. Verifying the seal may be done optionally here, or explicitly
	// via the VerifySeal method.
	VerifyHeader(chain ChainReader, header *types.Header, seal bool, mode config.SyncMode) error

	// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
	// concurrently. The method returns a quit channel to abort the operations and
	// a results channel to retrieve the async verifications (the order is that of
	// the input slice).
	VerifyHeaders(chain ChainReader, headers []*types.Header, seals []bool, mode config.SyncMode) (chan<- struct{}, <-chan error)

	// APIs returns the RPC APIs this consensus engine provides.
	APIs(chain ChainReader) []rpc.API
}
