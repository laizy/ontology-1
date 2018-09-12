/*
 * Copyright (C) 2018 The ontology Authors
 * This file is part of The ontology library.
 *
 * The ontology is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * The ontology is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public License
 * along with The ontology.  If not, see <http://www.gnu.org/licenses/>.
 */

package overlaydb

import (
	"crypto/sha256"

	"encoding/binary"
	comm "github.com/ontio/ontology/common"
	"github.com/ontio/ontology/common/log"
	"github.com/ontio/ontology/core/store/common"
	"github.com/syndtr/goleveldb/leveldb/util"
	"sort"
	"strings"
)

type OverlayDB struct {
	store common.PersistStore
	memdb *MemDB
	dbErr error
}

const initCap = 4 * 1024 * 1024
const initkvNum = 1024

func NewOverlayDB(store common.PersistStore) *OverlayDB {
	return &OverlayDB{
		store: store,
		memdb: NewMemDB(initCap, initkvNum),
	}
}

func (self *OverlayDB) Reset() {
	self.memdb.Reset()
}

func (self *OverlayDB) Error() error {
	return self.dbErr
}

func (self *OverlayDB) SetError(err error) {
	self.dbErr = err
}

// if key is deleted, value == nil
func (self *OverlayDB) Get(key []byte) (value []byte, err error) {
	var unknown bool
	value, unknown = self.memdb.Get(key)
	if unknown == false {
		return value, nil
	}

	value, err = self.store.Get(key)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		self.dbErr = err
		return nil, err
	}

	return
}

func (self *OverlayDB) Put(key []byte, value []byte) {
	self.memdb.Put(key, value)
}

func (self *OverlayDB) Delete(key []byte) {
	self.memdb.Delete(key)
}

func (self *OverlayDB) CommitTo(height uint32) []byte {
	var kv []string
	self.memdb.ForEach(func(key, val []byte) {
		if len(val) == 0 {
			var buf [4]byte
			binary.BigEndian.PutUint32(buf[:], uint32(len(key)))
			kv = append(kv, string(append(buf[:], []byte(key)...)))
			self.store.BatchDelete(key)
		} else {
			var buf [4]byte
			binary.BigEndian.PutUint32(buf[:], uint32(len(key)))
			item := string(append(buf[:], []byte(key)...))
			binary.BigEndian.PutUint32(buf[:], uint32(len(val)))
			item += string(append(buf[:], val...))
			kv = append(kv, item)
			self.store.BatchPut(key, val)
		}
	})

	sort.Strings(kv)
	kvall := strings.Join(kv, "")
	hash := sha256.Sum256([]byte(kvall))
	if height == 7180 {
		log.Fatalf("diff at height:%d, kvAll:%x", height, []byte(kvall))
	}
	return hash[:]
}

func (self *OverlayDB) GetWriteSet() *MemDB {
	return self.memdb
}

func (self *OverlayDB) ChangeHash() comm.Uint256 {
	stateDiff := sha256.New()
	self.memdb.ForEach(func(key, val []byte) {
		stateDiff.Write(key)
		stateDiff.Write(val)
	})

	var hash comm.Uint256
	stateDiff.Sum(hash[:0])
	return hash
}

// param key is referenced by iterator
func (self *OverlayDB) NewIterator(key []byte) common.StoreIterator {
	prefixRange := util.BytesPrefix(key)
	backIter := self.store.NewIterator(key)
	memIter := self.memdb.NewIterator(prefixRange)

	return NewJoinIter(memIter, backIter)
}
