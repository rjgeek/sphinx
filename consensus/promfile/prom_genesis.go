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

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"math/rand"
	"time"
	//"encoding/hex"

	"github.com/shx-project/sphinx/blockchain"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/config"
)

// generate genesis file with user input.
func (p *prometh) makeGenesis() {

	genesis := &bc.Genesis{
		Timestamp:  uint64(time.Now().Unix() / 1000 / 1000),
		Difficulty: big.NewInt(1),
		Config:     &config.ChainConfig{},
	}
	// Figure out which consensus engine to choose
	fmt.Println()
	fmt.Println("Welcome to SHX consensus engine file maker")

	genesis.Difficulty = big.NewInt(1)
	genesis.Config.Prometheus = &config.PrometheusConfig{
		Period: 3,
		Epoch:  30000,
	}
	fmt.Println()
	fmt.Println("How many seconds should blocks take? (default = 3)")
	genesis.Config.Prometheus.Period = uint64(p.readDefaultInt(3))

	fmt.Println()
	fmt.Println("How many blocks should voting epoch be ? (default = 30000)")
	genesis.Config.Prometheus.Epoch = uint64(p.readDefaultInt(30000))

	fmt.Println()
	fmt.Println("Specify your chain/network ID if you want an explicit one (default = random)")
	genesis.Config.ChainId = new(big.Int).SetUint64(uint64(p.readDefaultInt(rand.Intn(65536))))

	fmt.Println()
	fmt.Println("Anything fun to embed into the genesis block? (max 32 bytes)")

	extra := p.read()
	if len(extra) > 32 {
		extra = extra[:32]
	}
	genesis.ExtraData = append(genesis.ExtraData, extra...)

	p.conf.genesis = genesis
}

func (p *prometh) manageGenesis() {
	// Figure out whether to modify or export the genesis
	fmt.Println()
	fmt.Println(" 1. Export genesis configuration")

	choice := p.read()
	switch {
	case choice == "1":
		fmt.Println()
		fmt.Printf("Which file to save the genesis into? (default = %s.json)\n", p.network)
		out, _ := json.MarshalIndent(p.conf.genesis, "", "  ")

		fmt.Printf("%s", out)
		if err := ioutil.WriteFile(p.readDefaultString(fmt.Sprintf("%s.json", p.network)), out, 0644); err != nil {
			log.Error("Failed to save genesis file", "err", err)
		}
		log.Info("Exported existing genesis block")

	default:
		log.Error("That's not something I can do")
	}
}
