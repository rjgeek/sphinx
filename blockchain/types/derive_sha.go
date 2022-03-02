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

package types

import (
	"bytes"
	"fmt"
	"github.com/shx-project/sphinx/common/merkletree"

	"github.com/shx-project/sphinx/common"
	"github.com/shx-project/sphinx/common/rlp"
	"github.com/shx-project/sphinx/common/trie"
)

type DerivableList interface {
	Len() int
	GetRlp(i int) []byte
	GetMerkleContent(i int) merkletree.Content
}

func DeriveShaWithTree(list DerivableList) common.Hash {
	keybuf := new(bytes.Buffer)
	trie := new(trie.Trie)
	for i := 0; i < list.Len(); i++ {
		keybuf.Reset()
		rlp.Encode(keybuf, uint(i))
		trie.Update(keybuf.Bytes(), list.GetRlp(i))
	}
	return trie.Hash()
}

func DeriveSha(list DerivableList) common.Hash {
	if list.Len() == 0 {
		return EmptyRootHash
	}

	rootHash := common.Hash{}
	clist := make([]merkletree.Content,list.Len())
	for i:=0; i < list.Len(); i++ {
		clist[i] = list.GetMerkleContent(i)
	}

	t,err := merkletree.NewTree(clist)
	if err != nil {
		panic(fmt.Sprintf("merkletree.New tree panic:%s\n", err.Error()))
	}
	root := t.MerkleRoot()
	rootHash.SetBytes(root)
	return rootHash
}
