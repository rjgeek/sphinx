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

package p2p

import (
	"bytes"
	"errors"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/config"
	"github.com/shx-project/sphinx/network/p2p/discover"
	"math/big"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errClosed        = errors.New("peer set is closed")
	errNotRegistered = errors.New("peer is not registered")
	errIncomplete    = errors.New("PeerManager is incomplete creation")
)

const (
	maxKnownTxs    = 1000000 // Maximum transactions hashes to keep in the known list (prevent DOS)
	maxKnownBlocks = 100000  // Maximum block hashes to keep in the known list (prevent DOS)
)

type PeerManager struct {
	peers  map[string]*Peer //current peers list
	boots  map[string]*Peer //current boost list
	lock   sync.RWMutex
	closed bool

	server *Server   // pointer to server of p2p
	shxpro *ShxProto // pointer to shx protocol

}

var INSTANCE = atomic.Value{}

func PeerMgrInst() *PeerManager {
	if INSTANCE.Load() == nil {
		pm := &PeerManager{
			peers:  make(map[string]*Peer),
			boots:  make(map[string]*Peer),
			server: &Server{},
			shxpro: NewProtos(),
		}
		INSTANCE.Store(pm)
	}

	return INSTANCE.Load().(*PeerManager)
}

func (prm *PeerManager) Start(coinbase common.Address) error {

	config := config.GetShxConfigInstance()

	prm.server.Config = Config{
		NAT:        config.Network.NAT,
		Name:       config.Network.Name,
		PrivateKey: config.Node.PrivateKey,
		NetworkId:  config.Node.NetworkId,
		ListenAddr: config.Network.ListenAddr,

		NetRestrict:     config.Network.NetRestrict,
		NodeDatabase:    config.Network.NodeDatabase,
		BootstrapNodes:  config.Network.BootstrapNodes,
		EnableMsgEvents: config.Network.EnableMsgEvents,

		Protocols: prm.shxpro.Protocols(),
	}

	prm.server.Config.CoinBase = coinbase
	log.Info("Set coinbase address by start", "address", coinbase.String(), "roletype", config.Network.RoleType)
	prm.shxpro.networkId = prm.server.NetworkId
	prm.shxpro.regMsgProcess(ReqNodesMsg, HandleReqNodesMsg)
	prm.shxpro.regMsgProcess(ResNodesMsg, HandleResNodesMsg)

	prm.shxpro.regMsgProcess(ReqRemoteStateMsg, HandleReqRemoteStateMsg)
	prm.shxpro.regMsgProcess(ResRemoteStateMsg, HandleResRemoteStateMsg)

	copy(prm.server.Protocols, prm.shxpro.Protocols())

	localType := discover.MineNode
	if config.Network.RoleType == "bootnode" {
		localType = discover.BootNode
	}
	prm.SetLocalType(localType)
	log.Info("Set Init Local Type by p2p", "type", localType.ToString())

	if err := prm.server.Start(); err != nil {
		log.Error("Shx protocol", "error", err)
		return err
	}
	////////////////////////////////////////////////////////////////////////////////////////
	//for bootnode check
	self := prm.server.Self()
	for _, n := range config.Network.BootstrapNodes {
		if self.ID == n.ID && prm.server.localType != discover.BootNode {
			panic("Need BOOTNODE flag.")
		}
	}

	//for bing info
	//if prm.server.localType == discover.BootNode {
	//	filename := filepath.Join(config.Node.DataDir, bindInfoFileName)
	//	log.Debug("bootnode load bindings", "filename", filename)
	//	prm.parseBindInfo(filename)
	//}

	return nil
}

func (prm *PeerManager) Stop() {
	prm.server.Stop()
	prm.server = nil

	prm.close()
	log.Info("Shx PeerManager stoped")

}

func (prm *PeerManager) P2pSvr() *Server {
	return prm.server
}

// Register injects a new peer into the working set, or returns an error if the
// peer is already known.
func (prm *PeerManager) Register(p *Peer) error {
	prm.lock.Lock()
	defer prm.lock.Unlock()

	if prm.closed {
		return errClosed
	}
	if p.remoteType == discover.BootNode {
		if _, ok := prm.boots[p.id]; !ok {
			prm.boots[p.id] = p
			log.Debug("Peer with bootnode is listed.")
		}
		return nil
	}

	if _, ok := prm.peers[p.id]; ok {
		return DiscAlreadyConnected
	}
	prm.peers[p.id] = p
	return nil
}

// Unregister removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (prm *PeerManager) unregister(id string) error {
	prm.lock.Lock()
	defer prm.lock.Unlock()

	if _, ok := prm.peers[id]; ok {
		delete(prm.peers, id)
	}

	if _, ok := prm.boots[id]; ok {
		delete(prm.boots, id)
	}

	return nil
}

// Peer retrieves the registered peer with the given id.
func (prm *PeerManager) Peer(id string) *Peer {
	prm.lock.RLock()
	defer prm.lock.RUnlock()

	return prm.peers[id]
}

func (prm *PeerManager) DefaultAddr() common.Address {
	return prm.server.CoinBase
}

func (prm *PeerManager) PeersAll() []*Peer {
	prm.lock.RLock()
	defer prm.lock.RUnlock()

	list := make([]*Peer, 0, len(prm.peers))
	for _, p := range prm.peers {
		list = append(list, p)
	}
	return list
}

func (prm *PeerManager) GetLocalType() discover.NodeType {
	return prm.server.localType
}

func (prm *PeerManager) SetLocalType(nt discover.NodeType) bool {
	log.Info("Change node local type", "from", prm.server.localType.ToString(), "to", nt.ToString())

	if prm.server.localType != nt {
		prm.lock.Lock()
		defer prm.lock.Unlock()

		prm.server.localType = nt
		for _, p := range prm.peers {
			p.localType = nt
		}
		return true
	}

	return false
}

// Len returns if the current number of peers in the set.
func (prm *PeerManager) Len() int {
	prm.lock.RLock()
	defer prm.lock.RUnlock()

	return len(prm.peers)
}

// PeersWithoutBlock retrieves a list of peers that do not have a given block in
// their set of known hashes.
func (prm *PeerManager) PeersWithoutBlock(hash common.Hash) []*Peer {
	prm.lock.RLock()
	defer prm.lock.RUnlock()

	list := make([]*Peer, 0, len(prm.peers))
	for _, p := range prm.peers {
		if !p.knownBlocks.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

// PeersWithoutTx retrieves a list of peers that do not have a given transaction
// in their set of known hashes.
func (prm *PeerManager) PeersWithoutTx(hash common.Hash) []*Peer {
	prm.lock.RLock()
	defer prm.lock.RUnlock()

	list := make([]*Peer, 0, len(prm.peers))
	for _, p := range prm.peers {
		if !p.knownTxs.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

// BestPeer retrieves the known peer with the currently highest total difficulty.
func (prm *PeerManager) BestPeer() *Peer {
	prm.lock.RLock()
	defer prm.lock.RUnlock()

	var (
		bestPeer *Peer
		bestTd   *big.Int
	)
	for _, p := range prm.peers {
		if p.msgLooping == false {
			continue
		}

		if _, td := p.Head(); bestPeer == nil || td.Cmp(bestTd) > 0 {
			bestPeer, bestTd = p, td
		}
	}
	return bestPeer
}

// Close disconnects all peers.
// No new peers can be registered after Close has returned.
func (prm *PeerManager) close() {
	prm.lock.Lock()
	defer prm.lock.Unlock()

	for _, p := range prm.peers {
		p.Disconnect(DiscQuitting)
	}
	prm.closed = true
}

func (prm *PeerManager) Protocol() []Protocol {
	return prm.shxpro.protos
}

////////////////////////////////////////////////////////////////////

type PeerInfo struct {
	ID       string `json:"id"`       // Unique node identifier (also the encryption key)
	Name     string `json:"name"`     // Name of the node, including client type, version, OS, custom data
	Version  string `json:"version"`  // Gshx version
	Remote   string `json:"remote"`   // Remote node type
	CoinBase string `json:"coinbase"` //Remote Node's CoinBase
	Cap      string `json:"cap"`      // Sum-protocols advertised by this particular peer
	Network  struct {
		Local  string `json:"local"`  // Local endpoint of the TCP data connection
		Remote string `json:"remote"` // Remote endpoint of the TCP data connection
	} `json:"network"`
	Start  string      `json:"start"`  //
	Beat   string      `json:"beat"`   //
	Mining string      `json:"mining"` //
	SHX    interface{} `json:"shx"`    // Sub-protocol specific metadata fields
}

type ShxInfo struct {
	TD   *big.Int `json:"handshakeTD"` // Total difficulty of the peer's blockchain
	Head string   `json:"handshakeHD"` // SHA3 hash of the peer's best owned block
}

func (prm *PeerManager) PeerWithAddr(addr common.Address) *Peer {
	prm.lock.RLock()
	defer prm.lock.RUnlock()
	for _, p := range prm.peers {
		if p.remoteType == discover.MineNode && bytes.Compare(p.Address().Bytes(), addr.Bytes()) == 0 {
			return p
		}
	}
	return nil
}

func (prm *PeerManager) PeersInfo() []*PeerInfo {
	prm.lock.RLock()
	defer prm.lock.RUnlock()

	req := statusRes{Version: 0x01}
	req.Status = append(req.Status, StatDetail{0x00, ""})
	for _, p := range prm.peers {
		if p.remoteType == discover.MineNode {
			SendData(p, ReqRemoteStateMsg, req)
		}
	}
	time.Sleep(time.Second)

	allinfos := make([]*PeerInfo, 0, len(prm.boots)+len(prm.peers))
	for _, p := range prm.boots {
		info := &PeerInfo{
			ID:       p.ID().TerminalString(),
			Name:     p.Name(),
			Version:  p.Version(),
			Remote:   p.remoteType.ToString(),
			CoinBase: p.Address().String(),
			Cap:      p.Caps()[0].String(),
			Start:    p.beatStart.String(),
			Beat:     strconv.FormatUint(p.count, 10),
			Mining:   p.statMining,
			SHX:      "",
		}
		info.Network.Local = p.LocalAddr().String()
		info.Network.Remote = p.RemoteAddr().String()

		allinfos = append(allinfos, info)
	}

	peerinfos := make([]*PeerInfo, 0, len(prm.peers))
	for _, p := range prm.peers {
		hash, td := p.Head()
		info := &PeerInfo{
			ID:      p.ID().TerminalString(),
			Name:    p.Name(),
			Version: p.Version(),
			Remote:  p.remoteType.ToString(),
			Cap:     p.Caps()[0].String(),
			Start:   p.beatStart.String(),
			Beat:    strconv.FormatUint(p.count, 10),
			Mining:  p.statMining,
			SHX: &ShxInfo{
				TD:   td,
				Head: hash.Hex(),
			},
		}
		info.Network.Local = p.LocalAddr().String()
		info.Network.Remote = p.RemoteAddr().String()
		peerinfos = append(peerinfos, info)
	}

	for i := 0; i < len(peerinfos); i++ {
		for j := i + 1; j < len(peerinfos); j++ {
			if peerinfos[i].ID > peerinfos[j].ID {
				peerinfos[i], peerinfos[j] = peerinfos[j], peerinfos[i]
			}
		}
	}
	allinfos = append(allinfos, peerinfos...)

	return allinfos
}

type NodeInfo struct {
	ID    string `json:"id"`    // Unique node identifier (also the encryption key)
	Name  string `json:"name"`  // Name of the node, including client type, version, OS, custom data
	Local string `json:"local"` // Local node type
	IP    string `json:"ip"`    // IP address of the node
	Ports struct {
		UDP int `json:"udp"` // UDP listening port for discovery protocol
		TCP int `json:"tcp"` // TCP listening port for RLPx
	} `json:"ports"`
	ListenAddr string `json:"listenAddr"`
}

func (prm *PeerManager) NodeInfo() *NodeInfo {
	node := prm.server.Self()

	info := &NodeInfo{
		Name:       prm.server.Name,
		Local:      prm.server.localType.ToString(),
		ID:         node.ID.String(),
		IP:         node.IP.String(),
		ListenAddr: prm.server.ListenAddr,
	}
	info.Ports.UDP = int(node.UDP)
	info.Ports.TCP = int(node.TCP)

	return info
}

////////////////////////////////////////////////////////////////////
func (prm *PeerManager) RegMsgProcess(msg uint64, cb MsgProcessCB) {
	prm.shxpro.regMsgProcess(msg, cb)
	return
}

func (prm *PeerManager) RegChanStatus(cb ChanStatusCB) {
	prm.shxpro.regChanStatus(cb)
	log.Debug("ChanStatus has been register")
	return
}

func (prm *PeerManager) RegOnAddPeer(cb OnAddPeerCB) {
	prm.shxpro.regOnAddPeer(cb)
	log.Debug("OnAddPeer has been register")
	return
}

func (prm *PeerManager) RegOnDropPeer(cb OnDropPeerCB) {
	prm.shxpro.regOnDropPeer(cb)
	log.Debug("OnDropPeer has been register")
	return
}

func (prm *PeerManager) RegStatMining(cb StatMining) {
	prm.shxpro.regStatMining(cb)
	log.Debug("StatMining has been register")
	return
}
