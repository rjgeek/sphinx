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

package synctrl

import (
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/network/p2p"
	"github.com/shx-project/sphinx/network/p2p/discover"
)

// routingTx will propagate a transaction to peers by type which are not known to
// already have the given transaction.
func routTx(hash common.Hash, tx *types.Transaction) {
	// Broadcast transaction to a batch of peers not knowing about it

	if tx.IsForward() {
		//routForwardTx(hash,tx)
	} else {
		tx.SetForward(true)
		routNativeTx(hash, tx)
	}
}

func routNativeTx(hash common.Hash, tx *types.Transaction) {
	peers := p2p.PeerMgrInst().PeersWithoutTx(hash)
	if len(peers) == 0 {
		return
	}

	switch p2p.PeerMgrInst().GetLocalType() {
	case discover.MineNode:
		for _, peer := range peers {
			switch peer.RemoteType() {
			case discover.MineNode:
				sendTransactions(peer, types.Transactions{tx})
				break
			}
		}
		break
	}
	log.Trace("Broadcast transaction", "hash", hash, "recipients", len(peers))
}

func sendTransactions(peer *p2p.Peer, txs types.Transactions) error {
	for _, tx := range txs {
		log.Trace("route ", "tx", tx.Hash(), "to peer", peer.String())
		peer.KnownTxsAdd(tx.Hash())
	}
	return p2p.SendData(peer, p2p.TxMsg, txs)
}

func routProof(proof types.WorkProofMsg) {
	peers := p2p.PeerMgrInst().PeersAll()
	for _, peer := range peers {
		if peer.RemoteType() != discover.BootNode {
			p2p.SendData(peer, p2p.WorkProofMsg, proof)
		}
	}
}

func routProofConfirm(confirm types.ConfirmMsg) {
	peers := p2p.PeerMgrInst().PeersAll()
	for _, peer := range peers {
		if peer.RemoteType() != discover.BootNode {
			p2p.SendData(peer, p2p.ProofConfirmMsg, confirm)
		}
	}
}

func routQueryState(query types.QueryStateMsg) {
	peers := p2p.PeerMgrInst().PeersAll()
	for _, peer := range peers {
		if peer.RemoteType() != discover.BootNode {
			p2p.SendData(peer, p2p.GetStateMsg, query)
		}
	}
}

func routResponseState(response types.ResponseStateMsg) {
	peers := p2p.PeerMgrInst().PeersAll()
	for _, peer := range peers {
		if peer.RemoteType() != discover.BootNode {
			p2p.SendData(peer, p2p.ResStateMsg, response)
		}
	}
}
