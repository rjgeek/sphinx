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

package config

import (
	"bufio"
	"errors"
	"reflect"
	"unicode"

	"fmt"
	"github.com/naoina/toml"
	"github.com/shx-project/sphinx/common/log"
	"os"
	"sync/atomic"
)

var ShxConfigIns *ShxConfig

const (
	DatadirPrivateKey      = "nodekey"            // Path within the datadir to the node's private key
	DatadirDefaultKeyStore = "keystore"           // Path within the datadir to the keystore
	DatadirStaticNodes     = "static-nodes.json"  // Path within the datadir to the static node list
	DatadirTrustedNodes    = "trusted-nodes.json" // Path within the datadir to the trusted node list
	DatadirNodeDatabase    = "nodes"              // Path within the datadir to store the node infos
)

const (
	MaximumExtraDataSize uint64 = 32 // Maximum size extra data may be after Genesis.

	EpochDuration   uint64 = 30000 // Duration between proof-of-work epochs.
	CallCreateDepth uint64 = 1024  // Maximum depth of call/create stack.
	StackLimit      uint64 = 1024  // Maximum size of VM stack allowed.
)

// config instance
var INSTANCE = atomic.Value{}

type shxStatsConfig struct {
	URL string `toml:",omitempty"`
}

// Config represents a small collection of configuration values to fine tune the
// P2P network layer of a protocol stack. These values can be further extended by
// all registered services.
type ShxConfig struct {
	Node Nodeconfig
	// Configuration of peer-to-peer networking.
	Network NetworkConfig

	//configuration of txpool
	TxPool TxPoolConfiguration

	//configuration of blockchain
	BlockChain ChainConfig

	//configuration of consensus
	Prometheus PrometheusConfig

	ShxStats shxStatsConfig
}

// These settings ensure that TOML keys use the same names as Go struct fields.
var tomlSettings = toml.Config{
	NormFieldName: func(rt reflect.Type, key string) string {
		return key
	},
	FieldToKey: func(rt reflect.Type, field string) string {
		return field
	},
	MissingField: func(rt reflect.Type, field string) error {
		link := ""
		if unicode.IsUpper(rune(rt.Name()[0])) && rt.PkgPath() != "main" {
			link = fmt.Sprintf(", see https://godoc.org/%s#%s for available fields", rt.PkgPath(), rt.Name())
		}
		return fmt.Errorf("field '%s' is not defined in %s%s", field, rt.String(), link)
	},
}

func loadConfig(file string, cfg *ShxConfig) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	err = tomlSettings.NewDecoder(bufio.NewReader(f)).Decode(cfg)
	// Add file name to errors that have a line number.
	if _, ok := err.(*toml.LineError); ok {
		err = errors.New(file + ", " + err.Error())
	}
	return err
}
func New() *ShxConfig {
	if INSTANCE.Load() != nil {
		return INSTANCE.Load().(*ShxConfig)
	}

	if ShxConfigIns == nil {
		ShxConfigIns := &ShxConfig{
			Node: defaultNodeConfig(),
			// Configuration of peer-to-peer networking.
			Network: DefaultNetworkConfig(),

			//configuration of txpool
			TxPool: DefaultTxPoolConfig,

			//configuration of blockchain
			BlockChain: DefaultBlockChainConfig,
			//configuration of consensus
			Prometheus: DefaultPrometheusConfig,
		}
		log.Info("Create New ShxConfig object")
		INSTANCE.Store(ShxConfigIns)
		return ShxConfigIns
	}

	INSTANCE.Store(ShxConfigIns)
	return ShxConfigIns

}
func GetShxConfigInstance() *ShxConfig {
	if INSTANCE.Load() != nil {
		return INSTANCE.Load().(*ShxConfig)
	}
	ShxConfigIns := &ShxConfig{
		Node: defaultNodeConfig(),
		// Configuration of peer-to-peer networking.
		Network: DefaultNetworkConfig(),

		//configuration of txpool
		TxPool: DefaultTxPoolConfig,

		//configuration of blockchain
		BlockChain: DefaultBlockChainConfig,
		//configuration of consensus
		Prometheus: DefaultPrometheusConfig,
	}
	log.Info("Create New ShxConfig object")
	INSTANCE.Store(ShxConfigIns)
	return INSTANCE.Load().(*ShxConfig)
}
