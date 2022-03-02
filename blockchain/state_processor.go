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

package bc

import (
	"github.com/shx-project/sphinx/blockchain/state"
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/config"
	"github.com/shx-project/sphinx/consensus"
)

// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type StateProcessor struct {
	config *config.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}

// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(config *config.ChainConfig, bc *BlockChain, engine consensus.Engine) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// Process processes the state changes according to the Shx rules by running
// the transaction messages using the statedb.
//
// Process returns the receipts and logs accumulated during the process.
func (p *StateProcessor) Process(block *types.Block, statedb *state.StateDB) (types.Receipts, []*types.Log, error) {
	var (
		receipts types.Receipts
		receipt  *types.Receipt
		errs     error
		header   = block.Header()
	)
	// Iterate over and process the individual transactions
	author, _ := p.engine.Author(block.Header())
	for i, tx := range block.Transactions() {
		statedb.Prepare(tx.Hash(), block.Hash(), i)

		receipt, errs = ApplyTransaction(p.config, p.bc, &author, statedb, header, tx)
		if errs != nil {
			return nil, nil, errs
		}
		receipts = append(receipts, receipt)
	}

	// Finalize the block, applying any consensus engine specific extras.
	if _, errfinalize := p.engine.Finalize(p.bc, header, statedb, block.Transactions(), block.Proofs(), receipts); nil != errfinalize {
		return nil, nil, errfinalize
	}

	return receipts, nil, nil
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction and an error if the transaction failed,
// indicating the block was invalid.
func ApplyTransaction(config *config.ChainConfig, bc *BlockChain, author *common.Address, statedb *state.StateDB, header *types.Header, tx *types.Transaction) (*types.Receipt, error) {
	// milestone 1, not need apply tx.
	var root []byte
	receipt := types.NewReceipt(root,false)
	receipt.TxHash = tx.Hash()
	receipt.ConfirmCount = 1
	return receipt, nil
}
