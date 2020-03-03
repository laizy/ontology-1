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

package keyspace

import (
	"sort"

	"math/big"
)

// Key represents an identifier in a KeySpace. It holds a reference to the
// associated KeySpace, as well references to both the Original identifier,
// as well as the new, KeySpace Bytes one.
type Key struct {
	// Space is the KeySpace this Key is related to.
	Space KeySpace

	// Original is the original value of the identifier
	Original uint64

	// Bytes is the new value of the identifier, in the KeySpace.
	Bytes []byte
}

// Equal returns whether this key is equal to another.
func (k1 Key) Equal(k2 Key) bool {
	if k1.Space != k2.Space {
		panic("k1 and k2 not in same key space.")
	}
	return k1.Space.Equal(k1, k2)
}

// Less returns whether this key comes before another.
func (k1 Key) Less(k2 Key) bool {
	if k1.Space != k2.Space {
		panic("k1 and k2 not in same key space.")
	}
	return k1.Space.Less(k1, k2)
}

// Distance returns this key's distance to another
func (k1 Key) Distance(k2 Key) *big.Int {
	if k1.Space != k2.Space {
		panic("k1 and k2 not in same key space.")
	}
	return k1.Space.Distance(k1, k2)
}

// KeySpace is an object used to do math on identifiers. Each keyspace has its
// own properties and rules. See XorKeySpace.
type KeySpace interface {
	// Key converts an peer id into a Key in this space.
	Key(uint64) Key

	// Equal returns whether keys are equal in this key space
	Equal(Key, Key) bool

	// Distance returns the distance metric in this key space
	Distance(Key, Key) *big.Int

	// Less returns whether the first key is smaller than the second.
	Less(Key, Key) bool
}

// byDistanceToCenter is a type used to sort Keys by proximity to a center.
type byDistanceToCenter struct {
	Center Key
	Keys   []Key
}

func (s byDistanceToCenter) Len() int {
	return len(s.Keys)
}

func (s byDistanceToCenter) Swap(i, j int) {
	s.Keys[i], s.Keys[j] = s.Keys[j], s.Keys[i]
}

func (s byDistanceToCenter) Less(i, j int) bool {
	a := s.Center.Distance(s.Keys[i])
	b := s.Center.Distance(s.Keys[j])
	return a.Cmp(b) == -1
}

// SortByDistance takes a KeySpace, a center Key, and a list of Keys toSort.
// It returns a new list, where the Keys toSort have been sorted by their
// distance to the center Key.
func SortByDistance(sp KeySpace, center Key, toSort []Key) []Key {
	toSortCopy := make([]Key, len(toSort))
	copy(toSortCopy, toSort)
	bdtc := &byDistanceToCenter{
		Center: center,
		Keys:   toSortCopy, // copy
	}
	sort.Sort(bdtc)
	return bdtc.Keys
}