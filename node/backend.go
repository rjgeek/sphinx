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

// Package eth implements the Shx protocol.
package node

import (
	"github.com/shx-project/sphinx/account"
	"github.com/shx-project/sphinx/blockchain"
	"github.com/shx-project/sphinx/blockchain/storage"
	"github.com/shx-project/sphinx/consensus"
	"github.com/shx-project/sphinx/internal/shxapi"
	"github.com/shx-project/sphinx/network/p2p"
	"github.com/shx-project/sphinx/network/rpc"
	"github.com/shx-project/sphinx/node/filters"
	"github.com/shx-project/sphinx/txpool"
	"github.com/shx-project/sphinx/worker"
)

type LesServer interface {
	Start(srvr *p2p.Server)
	Stop()
	Protocols() []p2p.Protocol
}

// APIs returns the collection of RPC services the shx package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Node) APIs() []rpc.API {
	apis := shxapi.GetAPIs(s.ApiBackend)

	// Append all the local APIs and return
	apis = append(apis, []rpc.API{
		{
			Namespace: "shx",
			Version:   "1.0",
			Service:   NewPublicShxAPI(s),
			Public:    true,
		}, {
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewPrivateMinerAPI(s),
			Public:    false,
		}, {
			Namespace: "shx",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.ApiBackend, false),
			Public:    true,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(s),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(&s.Shxconfig.BlockChain, s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   shxapi.NewPublicNetAPI(p2p.PeerMgrInst().P2pSvr(), s.networkId), //s.netRPCService,
			Public:    true,
		},
	}...)

	// Append any APIs exposed explicitly by the consensus engine
	if s.Shxengine != nil {
		apis = append(apis, s.Shxengine.APIs(s.BlockChain())...)
	}
	return apis
}

func (s *Node) StopMining()          { s.miner.Stop() }
func (s *Node) IsMining() bool       { return s.miner.Mining() }
func (s *Node) Miner() *worker.Miner { return s.miner }

func (s *Node) APIAccountManager() *accounts.Manager { return s.accman }
func (s *Node) BlockChain() *bc.BlockChain           { return s.Shxbc }
func (s *Node) TxPool() *txpool.TxPool               { return s.Shxtxpool }
func (s *Node) Engine() consensus.Engine             { return s.Shxengine }
func (s *Node) ChainDb() shxdb.Database              { return s.ShxDb }
func (s *Node) IsListening() bool                    { return true } // Always listening
func (s *Node) ShxVersion() int                      { return int(s.Shxpeermanager.Protocol()[0].Version) }
func (s *Node) NetVersion() uint64                   { return s.networkId }
