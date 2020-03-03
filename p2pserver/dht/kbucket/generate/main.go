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

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

const bits = 16
const target = 1 << bits

func main() {
	file := os.Getenv("GOFILE")
	targetFile := strings.TrimSuffix(file, ".go") + "_prefixmap.go"

	ids := new([target]uint32)
	found := new([target]bool)
	count := int32(0)

	out := make([]byte, 32)
	inp := [32]byte{}
	hasher := sha256.New()

	for i := uint32(0); count < target; i++ {
		// must equal to ConvertPeerID
		binary.BigEndian.PutUint32(inp[:], i)
		hasher.Write(inp[:])
		out = hasher.Sum(out[:0])
		hasher.Reset()

		// hash value prefix
		prefix := binary.BigEndian.Uint32(out) >> (32 - bits)
		if !found[prefix] {
			found[prefix] = true
			ids[prefix] = i
			count++
		}
	}

	f, err := os.Create(targetFile)
	if err != nil {
		panic(err)
	}

	printf := func(s string, args ...interface{}) {
		_, err := fmt.Fprintf(f, s, args...)
		if err != nil {
			panic(err)
		}
	}

	printf("package kbucket\n\n")
	printf("// Code generated by generate/generate_map.go DO NOT EDIT\n")
	printf("var keyPrefixMap = [...]uint32{")
	for i, j := range ids[:] {
		if i%16 == 0 {
			printf("\n\t")
		} else {
			printf(" ")
		}
		printf("%d,", j)
	}
	printf("\n}")
	f.Close()
}