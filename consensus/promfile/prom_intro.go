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
// along with the sphinx. If not, see <http://wwp.gnu.org/licenses/>.

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/shx-project/sphinx/common/log"
)

// makeWizard creates and returns a new prometh prometh.
func makePrometh(network string) *prometh {
	return &prometh{
		network: network,
		conf: pconfig{
			Servers: make(map[string][]byte),
		},
		servers:  make(map[string]*sshClient),
		services: make(map[string][]string),
		in:       bufio.NewReader(os.Stdin),
	}
}

// run displays some useful infos to the user, starting on the journey of
// setting up a new or managing an existing SHX private network.
func (p *prometh) run() {
	fmt.Println("+-----------------------------------------------------------+")
	fmt.Println("| Welcome to promfile, your SHX private network manager     |")
	fmt.Println("|                                                           |")
	fmt.Println("| This tool lets you create a new SHX network down to the   |")
	fmt.Println("| genesis block, bootnodes, miners.                         |")
	fmt.Println("|                                                           |")
	fmt.Println("| Promfile uses SSH to dial in to remote servers, and builds |")
	fmt.Println("| its network components out of Docker containers using the |")
	fmt.Println("| docker-compose toolset.                                   |")
	fmt.Println("+-----------------------------------------------------------+")
	fmt.Println()

	// Make sure we have a good network name to work with	fmt.Println()
	if p.network == "" {
		fmt.Println("Please specify a network name to administer (no spaces, please)")
		for {
			p.network = p.readString()
			if !strings.Contains(p.network, " ") {
				fmt.Printf("Sweet, you can set this via --network=%s next time!\n\n", p.network)
				break
			}
			log.Error("I also like to live dangerously, still no spaces")
		}
	}
	log.Info("Administering SHX network", "name", p.network)

	// Load initial configurations and connect to all live servers
	p.conf.path = filepath.Join(os.Getenv("HOME"), ".promfile", p.network)

	blob, err := ioutil.ReadFile(p.conf.path)
	if err != nil {
		log.Warn("No previous configurations found", "path", p.conf.path)
	} else if err := json.Unmarshal(blob, &p.conf); err != nil {
		log.Crit("Previous configuration corrupted", "path", p.conf.path, "err", err)
	} else {
		for server, pubkey := range p.conf.Servers {
			log.Info("Dialing previously configured server", "server", server)
			client, err := dial(server, pubkey)
			if err != nil {
				log.Error("Previous server unreachable", "server", server, "err", err)
			}
			p.servers[server] = client
		}
		p.networkStats(false)
	}
	// Basics done, loop ad infinitum about what to do
	for {
		fmt.Println()
		fmt.Println("What would you like to do? (default = stats)")
		fmt.Println(" 1. Configure new genesis")
		fmt.Println(" 2. Manage existing genesis")

		choice := p.read()
		switch {
		case choice == "" || choice == "1":
			p.makeGenesis()

		case choice == "2":
			if p.conf.genesis == nil {
				log.Error("There is no genesis to manage")
			} else {
				p.manageGenesis()
			}
		default:
			log.Error("That's not something I can do")
		}
	}
}
