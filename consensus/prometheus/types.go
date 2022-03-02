package prometheus

import (
	"github.com/hashicorp/golang-lru"
	"github.com/shx-project/sphinx/account"
	"github.com/shx-project/sphinx/blockchain/storage"
	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/config"
	"github.com/shx-project/sphinx/node/db"
	"sync"
)

// constant parameter definition
const (
	inmemorySignatures = 4096
)

// Prometheus protocol constants.
var (
	epochLength   = uint64(30000)
	reentryMux    sync.Mutex
	insPrometheus *Prometheus
)

type Prometheus struct {
	config *config.PrometheusConfig // Consensus config
	db     shxdb.Database           // Database

	signatures *lru.ARCCache // the last signature

	signer common.Address
	signFn SignerFn     // Callback function
	lock   sync.RWMutex // Protects the signerHash fields
}

func New(config *config.PrometheusConfig, db shxdb.Database) *Prometheus {

	conf := *config

	if conf.Epoch == 0 {
		conf.Epoch = epochLength
	}

	signatures, _ := lru.NewARC(inmemorySignatures)

	return &Prometheus{
		config:     &conf,
		db:         db,
		signatures: signatures,
	}
}

// InstanceBlockChain returns the singleton of BlockChain.
func InstancePrometheus() *Prometheus {
	if nil == insPrometheus {
		reentryMux.Lock()
		if nil == insPrometheus {
			insPrometheus = New(&config.GetShxConfigInstance().Prometheus, db.GetShxDbInstance())
		}
		reentryMux.Unlock()
	}
	return insPrometheus
}

type SignerFn func(accounts.Account, []byte) ([]byte, error)
