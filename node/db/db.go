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

package db

import (
	"github.com/shx-project/sphinx/blockchain/storage"
	"github.com/shx-project/sphinx/common/log"
	"github.com/shx-project/sphinx/config"
	"sync/atomic"
)

// config instance
var DBINSTANCE = atomic.Value{}

// CreateDB creates the chain database.
func CreateDB(config *config.Nodeconfig, name string) (shxdb.Database, error) {

	if DBINSTANCE.Load() != nil {
		return DBINSTANCE.Load().(*shxdb.LDBDatabase), nil
	}
	db, err := OpenDatabase(name, config.DatabaseCache, config.DatabaseHandles)
	if err != nil {
		return nil, err
	}
	if db, ok := db.(*shxdb.LDBDatabase); ok {
		db.Meter("shx/db/chaindata/")
	}
	DBINSTANCE.Store(db)
	return db, nil
}

// OpenDatabase opens an existing database with the given name (or creates one
// if no previous can be found) from within the node's data directory. If the
// node is an ephemeral one, a memory database is returned.
func OpenDatabase(name string, cache int, handles int) (shxdb.Database, error) {

	if DBINSTANCE.Load() != nil {
		return DBINSTANCE.Load().(*shxdb.LDBDatabase), nil
	}

	var cfg = config.GetShxConfigInstance()
	if cfg.Node.DataDir == "" {
		return shxdb.NewMemDatabase()
	}
	db, err := shxdb.NewLDBDatabase(cfg.Node.ResolvePath(name), cache, handles)
	if err != nil {
		return nil, err
	}
	DBINSTANCE.Store(db)

	return db, nil
}

func GetShxDbInstance() *shxdb.LDBDatabase {
	if DBINSTANCE.Load() != nil {
		return DBINSTANCE.Load().(*shxdb.LDBDatabase)
	}
	log.Warn("LDBDatabase is nil, please init tx pool first.")
	return nil
}
