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

package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shx-project/sphinx/internal/shxapi"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/prometheus/prometheus/util/flock"
	"github.com/shx-project/sphinx/account"
	"github.com/shx-project/sphinx/account/keystore"
	"github.com/shx-project/sphinx/blockchain"
	"github.com/shx-project/sphinx/blockchain/bloombits"
	"github.com/shx-project/sphinx/blockchain/storage"
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/hexutil"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/common/rlp"
	"github.com/shx-project/sphinx/config"
	"github.com/shx-project/sphinx/consensus"
	"github.com/shx-project/sphinx/consensus/prometheus"
	"github.com/shx-project/sphinx/event/sub"
	"github.com/shx-project/sphinx/internal/debug"
	"github.com/shx-project/sphinx/network/p2p"
	"github.com/shx-project/sphinx/network/rpc"
	"github.com/shx-project/sphinx/node/db"
	"github.com/shx-project/sphinx/synctrl"
	"github.com/shx-project/sphinx/txpool"
	"github.com/shx-project/sphinx/worker"
)

// Node is a container on which services can be registered.
type Node struct {
	accman      *accounts.Manager
	newBlockMux *sub.TypeMux

	Shxconfig      *config.ShxConfig
	Shxpeermanager *p2p.PeerManager
	Shxrpcmanager  *rpc.RpcManager
	Shxsyncctr     *synctrl.SynCtrl
	Shxtxpool      *txpool.TxPool
	Shxbc          *bc.BlockChain
	//ShxDb
	ShxDb shxdb.Database

	networkId     uint64
	netRPCService *shxapi.PublicNetAPI

	Shxengine     consensus.Engine
	bloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer  *bc.ChainIndexer               // Bloom indexer operating during block imports

	// Channel for shutting down the service
	shutdownChan  chan bool    // Channel for shutting down the shx
	stopDbUpgrade func() error // stop chain db sequential key upgrade

	miner     *worker.Miner
	shxerbase common.Address

	ephemeralKeystore string         // if non-empty, the key directory that will be removed by Stop
	instanceDirLock   flock.Releaser // prevents concurrent use of instance directory

	rpcAPIs       []rpc.API   // List of APIs currently provided by the node
	inprocHandler *rpc.Server // In-process RPC request handler to process the API requests

	lock       sync.RWMutex
	ApiBackend *ShxApiBackend

	RpcAPIs []rpc.API // List of APIs currently provided by the node

	stop chan struct{} // Channel to wait for termination notifications

}

// New creates a shx node, create all object and start
func New(conf *config.ShxConfig) (*Node, error) {
	if conf.Node.DataDir != "" {
		absdatadir, err := filepath.Abs(conf.Node.DataDir)
		if err != nil {
			return nil, err
		}
		conf.Node.DataDir = absdatadir

	}

	// Ensure that the instance name doesn't cause weird conflicts with
	// other files in the data directory.
	if strings.ContainsAny(conf.Node.Name, `/\`) {
		return nil, errors.New(`Config.Name must not contain '/' or '\'`)
	}
	if conf.Node.Name == config.DatadirDefaultKeyStore {
		return nil, errors.New(`Config.Name cannot be "` + config.DatadirDefaultKeyStore + `"`)
	}
	if strings.HasSuffix(conf.Node.Name, ".ipc") {
		return nil, errors.New(`Config.Name cannot end in ".ipc"`)
	}

	hpbnode := &Node{
		Shxconfig:      conf,
		Shxpeermanager: nil, //peermanager,
		Shxsyncctr:     nil, //syncctr,
		Shxtxpool:      nil, //hpbtxpool,
		Shxbc:          nil, //block,

		ShxDb:     nil, //db,
		networkId: conf.Node.NetworkId,

		newBlockMux: nil,
		accman:      nil,
		Shxengine:   nil,

		shxerbase:     common.Address{},
		bloomRequests: make(chan chan *bloombits.Retrieval),
		bloomIndexer:  nil,
		stop:          make(chan struct{}),
	}
	log.Info("Initialising Shx node", "network", conf.Node.NetworkId)

	hpbdatabase, _ := db.CreateDB(&conf.Node, "chaindata")
	// Ensure that the AccountManager method works before the node has started.
	// We rely on this in cmd/geth.
	am, _, err := makeAccountManager(&conf.Node)
	if err != nil {
		return nil, err
	}
	hpbnode.accman = am

	if wallets := hpbnode.AccountManager().Wallets(); len(wallets) > 0 {
		if account := wallets[0].Accounts(); len(account) > 0 {
			hpbnode.shxerbase = account[0].Address
		}
		log.Info("Set coinbase with account[0]")
	} else {
		log.Info("Not Set coinbase")
	}

	// Note: any interaction with Config that would create/touch files
	// in the data directory or instance directory is delayed until Start.
	//create all object
	peermanager := p2p.PeerMgrInst()
	hpbnode.Shxpeermanager = peermanager
	hpbnode.Shxrpcmanager = rpc.RpcMgrInst()

	hpbnode.ShxDb = hpbdatabase

	hpbnode.newBlockMux = new(sub.TypeMux)

	hpbnode.Shxbc = bc.InstanceBlockChain()

	peermanager.RegChanStatus(hpbnode.Shxbc.Status)

	txpool.NewTxPool(conf.TxPool, &conf.BlockChain, hpbnode.Shxbc)
	hpbtxpool := txpool.GetTxPool()

	hpbnode.Shxtxpool = hpbtxpool
	hpbnode.ApiBackend = &ShxApiBackend{hpbnode}

	hpbnode.bloomIndexer = NewBloomIndexer(hpbdatabase, 4096)
	return hpbnode, nil
}
func (hpbnode *Node) WorkerInit(conf *config.ShxConfig) error {
	stored := bc.GetCanonicalHash(hpbnode.ShxDb, 0)
	if stored != (common.Hash{}) {
		if !conf.Node.SkipBcVersionCheck {
			bcVersion := bc.GetBlockChainVersion(hpbnode.ShxDb)
			if bcVersion != bc.BlockChainVersion && bcVersion != 0 {
				return fmt.Errorf("Blockchain DB version mismatch (%d / %d). Run geth upgradedb.\n", bcVersion, bc.BlockChainVersion)
			}
			bc.WriteBlockChainVersion(hpbnode.ShxDb, bc.BlockChainVersion)
		}
		engine := prometheus.InstancePrometheus()
		hpbnode.Shxengine = engine
		//add consensus engine to blockchain
		_, err := hpbnode.Shxbc.InitWithEngine(engine)
		if err != nil {
			log.Error("add engine to blockchain error")
			return err
		}
		hpbnode.Shxsyncctr = synctrl.InstanceSynCtrl()
		hpbnode.newBlockMux = hpbnode.Shxsyncctr.NewBlockMux()

		hpbnode.miner = worker.New(&conf.BlockChain, hpbnode.NewBlockMux(), hpbnode.Shxengine, hpbnode.shxerbase, hpbnode.ShxDb)
		hpbnode.bloomIndexer.Start(hpbnode.Shxbc.CurrentHeader(), hpbnode.Shxbc.SubscribeChainEvent)

	} else {
		return errors.New(`The genesis block is not inited`)
	}
	return nil
}

type ConsensuscfgF struct {
	HpNodesNum       int      //`json:"HpNodesNum"` 			//hp nodes number
	HpVotingRndScope int      //`json:"HpVotingRndScope"`		//hp voting rand selection scope
	FinalizeRetErrIg bool     //`json:"FinalizeRetErrIg"`	 	//finalize return err ignore
	Time             int      //`json:"Time"`					//gen block interval
	Nodeids          []string //`json:"Nodeids"`				//bootnode`s nodeid only add one
}

func parseConsensusConfigFile(conf *config.ShxConfig) {

	path := conf.Node.DataDir + "/" + conf.Node.FNameConsensusCfg
	_, err := os.Stat(path)
	if err != nil {
		if os.IsExist(err) {
			log.Info("parse consensus config file success", "err", err)
		} else {
			log.Warn("parse consensus config file fail", "err", err)
			return
		}
	}

	cfgfile := ConsensuscfgF{}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		log.Error("ioutil.ReadFile fail", "err", err)
		return
	}

	err = json.Unmarshal(data, &cfgfile)
	if err != nil {
		log.Error("json.Unmarshal fail", "err", err)
		return
	}

	consensus.IgnoreRetErr = cfgfile.FinalizeRetErrIg
	conf.Prometheus.Period = uint64(cfgfile.Time)

	config.MainnetBootnodes = config.MainnetBootnodes[:0]
	for _, v := range cfgfile.Nodeids {
		config.MainnetBootnodes = append(config.MainnetBootnodes, v)
	}
}

func (hpbnode *Node) Start(conf *config.ShxConfig) error {

	if conf.Node.FNameConsensusCfg != "" {
		parseConsensusConfigFile(conf)
	}
	log.Info("consensus.IgnoreRetErr", "value", consensus.IgnoreRetErr)
	log.Info("conf.Prometheus.Period", "value", conf.Prometheus.Period)
	for _, v := range config.MainnetBootnodes {
		log.Info("config.MainnetBootnodes", "value", v)
	}

	hpbnode.startBloomHandlers()

	err := hpbnode.WorkerInit(conf)
	if err != nil {
		log.Error("Worker init failed", ":", err)
		return err
	}
	if hpbnode.Shxsyncctr == nil {
		log.Error("syncctrl is nil")
		return errors.New("synctrl is nil")
	}
	hpbnode.Shxsyncctr.Start()
	retval := hpbnode.Shxpeermanager.Start(hpbnode.shxerbase)
	if retval != nil {
		log.Error("Start hpbpeermanager error")
		return errors.New(`start peermanager error ".ipc"`)
	}
	hpbnode.Shxpeermanager.RegStatMining(hpbnode.miner.Mining)

	hpbnode.SetNodeAPI()
	hpbnode.Shxrpcmanager.Start(hpbnode.RpcAPIs)
	hpbnode.Shxtxpool.Start()
	return nil
}

func makeAccountManager(conf *config.Nodeconfig) (*accounts.Manager, string, error) {
	scryptN := keystore.StandardScryptN
	scryptP := keystore.StandardScryptP
	if conf.UseLightweightKDF {
		scryptN = keystore.LightScryptN
		scryptP = keystore.LightScryptP
	}

	var (
		keydir    string
		ephemeral string
		err       error
	)
	switch {
	case filepath.IsAbs(conf.KeyStoreDir):
		keydir = conf.KeyStoreDir
	case conf.DataDir != "":
		if conf.KeyStoreDir == "" {
			keydir = filepath.Join(conf.DataDir, config.DatadirDefaultKeyStore)
		} else {
			keydir, err = filepath.Abs(conf.KeyStoreDir)
		}
	case conf.KeyStoreDir != "":
		keydir, err = filepath.Abs(conf.KeyStoreDir)
	default:
		// There is no datadir.
		keydir, err = ioutil.TempDir("", "shx-keystore")
		ephemeral = keydir
	}
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(keydir, 0700); err != nil {
		return nil, "", err
	}
	return accounts.NewManager(keystore.NewKeyStore(keydir, scryptN, scryptP)), ephemeral, nil
}

func (n *Node) openDataDir() error {
	if n.Shxconfig.Node.DataDir == "" {
		return nil // ephemeral
	}

	instdir := filepath.Join(n.Shxconfig.Node.DataDir, n.Shxconfig.Node.StringName())
	if err := os.MkdirAll(instdir, 0700); err != nil {
		return err
	}
	// Lock the instance directory to prevent concurrent use by another instance as well as
	// accidental use of the instance directory as a database.
	release, _, err := flock.New(filepath.Join(instdir, "LOCK"))
	if err != nil {
		return convertFileLockError(err)
	}
	n.instanceDirLock = release
	return nil
}

// Stop terminates a running node along with all it's services. In the node was
// not started, an error is returned.
func (n *Node) Stop() error {
	n.lock.Lock()
	defer n.lock.Unlock()

	//stop all modules
	n.Shxsyncctr.Stop()
	n.Shxtxpool.Stop()
	n.miner.Stop()
	n.Shxpeermanager.Stop()

	n.Shxrpcmanager.Stop()
	n.ShxDb.Close()

	// Release instance directory lock.
	if n.instanceDirLock != nil {
		if err := n.instanceDirLock.Release(); err != nil {
			log.Error("Can't release datadir lock", "err", err)
		}
		n.instanceDirLock = nil
	}

	// unblock n.Wait
	close(n.stop)

	// Remove the keystore if it was created ephemerally.
	var keystoreErr error
	if n.ephemeralKeystore != "" {
		keystoreErr = os.RemoveAll(n.ephemeralKeystore)
	}

	if keystoreErr != nil {
		return keystoreErr
	}
	return nil
}

// Wait blocks the thread until the node is stopped. If the node is not running
// at the time of invocation, the method immediately returns.
func (n *Node) Wait() {
	n.lock.RLock()

	stop := n.stop
	n.lock.RUnlock()

	<-stop
}

// Restart terminates a running node and boots up a new one in its place. If the
// node isn't running, an error is returned.
func (n *Node) Restart() error {
	if err := n.Stop(); err != nil {
		return err
	}
	if err := n.Start(config.ShxConfigIns); err != nil {
		return err
	}
	return nil
}

// Attach creates an RPC client attached to an in-process API handler.
func (n *Node) Attach(ipc *rpc.Server) (*rpc.Client, error) {
	n.lock.RLock()
	defer n.lock.RUnlock()
	if ipc == nil {
		return nil, ErrNodeStopped
	}
	n.inprocHandler = ipc
	return rpc.DialInProc(n.inprocHandler), nil
}

// RPCHandler returns the in-process RPC request handler.
func (n *Node) RPCHandler() (*rpc.Server, error) {
	n.lock.RLock()
	defer n.lock.RUnlock()

	if n.inprocHandler == nil {
		return nil, ErrNodeStopped
	}
	return n.inprocHandler, nil
}

// DataDir retrieves the current datadir used by the protocol stack.
// Deprecated: No files should be stored in this directory, use InstanceDir instead.
func (n *Node) DataDir() string {
	return n.Shxconfig.Node.DataDir
}

// InstanceDir retrieves the instance directory used by the protocol stack.
func (n *Node) InstanceDir() string {
	return n.Shxconfig.Node.InstanceDir()
}

// AccountManager retrieves the account manager used by the protocol stack.
func (n *Node) AccountManager() *accounts.Manager {
	return n.accman
}

// EventMux retrieves the event multiplexer used by all the network services in
// the current protocol stack.
func (n *Node) NewBlockMux() *sub.TypeMux {
	return n.newBlockMux
}

// ResolvePath returns the absolute path of a resource in the instance directory.
func (n *Node) ResolvePath(x string) string {
	return n.Shxconfig.Node.ResolvePath(x)
}

// apis returns the collection of RPC descriptors this node offers.
func (n *Node) Nodeapis() []rpc.API {
	return []rpc.API{
		{
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(n),
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPublicAdminAPI(n),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   debug.Handler,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(n),
			Public:    true,
		}, {
			Namespace: "web3",
			Version:   "1.0",
			Service:   NewPublicWeb3API(n),
			Public:    true,
		},
	}
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(config.VersionMajor<<16 | config.VersionMinor<<8 | config.VersionPatch),
			"geth",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > config.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", config.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

func (s *Node) ResetWithGenesisBlock(gb *types.Block) {
	s.Shxbc.ResetWithGenesisBlock(gb)
}

func (s *Node) Shxerbase() (eb common.Address, err error) {
	s.lock.RLock()
	hpberbase := s.shxerbase
	s.lock.RUnlock()

	if hpberbase != (common.Address{}) {
		return hpberbase, nil
	}
	if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			return accounts[0].Address, nil
		}
	}
	return common.Address{}, fmt.Errorf("hpberbase address must be explicitly specified")
}

// set in js console via admin interface or wrapper from cli flags
func (self *Node) SetShxerbase(hpberbase common.Address) {
	self.lock.Lock()
	self.shxerbase = hpberbase
	self.lock.Unlock()
}

func (s *Node) StartMining(local bool) error {
	//read coinbase from node
	eb := s.shxerbase

	if promeengine, ok := s.Shxengine.(*prometheus.Prometheus); ok {
		wallet, err := s.accman.Find(accounts.Account{Address: eb})
		if wallet == nil || err != nil {
			log.Error("Shxerbase account unavailable locally", "err", err)
			return fmt.Errorf("signer missing: %v", err)
		}
		promeengine.Authorize(eb, wallet.SignHash)
	} else {
		log.Error("Cannot start mining without prometheus", "err", s.Shxengine)
	}
	if local {
		// If local (CPU) mining is started, we can disable the transaction rejection
		// mechanism introduced to speed sync times. CPU mining on mainnet is ludicrous
		// so noone will ever hit this path, whereas marking sync done on CPU mining
		// will ensure that private networks work in single miner mode too.
		atomic.StoreUint32(&s.Shxsyncctr.AcceptTxs, 1)
	}
	go s.miner.Start(eb)
	return nil
}

func (s *Node) SetOpt(maxtxs, peorid int) {
	s.miner.SetOpt(maxtxs, peorid)
}

// get all rpc api from modules
func (n *Node) GetAPI() error {
	return nil
}

func (n *Node) SetNodeAPI() error {
	n.RpcAPIs = n.APIs()
	n.RpcAPIs = append(n.RpcAPIs, n.Nodeapis()...)
	return nil
}
