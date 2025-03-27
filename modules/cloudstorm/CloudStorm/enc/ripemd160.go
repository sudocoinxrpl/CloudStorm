// our_ripemd160.go
// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ripemd160 implements the RIPEMD-160 hash algorithm.
// This is our own pure-Go implementation with no external dependencies.
// Although RIPEMD-160 is deprecated for new applications, many legacy systems still depend on it.
package ripemd160

import (
	"crypto"
	"encoding/binary"
	"hash"
)

func init() {
	crypto.RegisterHash(crypto.RIPEMD160, New)
}

const Size = 20
const BlockSize = 64

const (
	_s0 = 0x67452301
	_s1 = 0xefcdab89
	_s2 = 0x98badcfe
	_s3 = 0x10325476
	_s4 = 0xc3d2e1f0
)

// digest represents the partial evaluation of a checksum.
type digest struct {
	s  [5]uint32       // current state
	x  [BlockSize]byte // data block buffer
	nx int             // number of bytes in buffer
	tc uint64          // total number of bytes processed
}

func (d *digest) Reset() {
	d.s[0], d.s[1], d.s[2], d.s[3], d.s[4] = _s0, _s1, _s2, _s3, _s4
	d.nx = 0
	d.tc = 0
}

// New returns a new hash.Hash computing the RIPEMD-160 checksum.
func New() hash.Hash {
	d := new(digest)
	d.Reset()
	return d
}

func (d *digest) Size() int      { return Size }
func (d *digest) BlockSize() int { return BlockSize }

func (d *digest) Write(p []byte) (nn int, err error) {
	nn = len(p)
	d.tc += uint64(nn)
	if d.nx > 0 {
		n := len(p)
		if n > BlockSize-d.nx {
			n = BlockSize - d.nx
		}
		copy(d.x[d.nx:], p[:n])
		d.nx += n
		if d.nx == BlockSize {
			block(d, d.x[:])
			d.nx = 0
		}
		p = p[n:]
	}
	n := block(d, p)
	p = p[n:]
	if len(p) > 0 {
		d.nx = copy(d.x[:], p)
	}
	return
}

func (d0 *digest) Sum(in []byte) []byte {
	// Make a copy of d0 so that caller can continue using it.
	d := *d0
	var tmp [BlockSize]byte
	tmp[0] = 0x80
	var padLen int
	if d.tc%BlockSize < 56 {
		padLen = int(56 - d.tc%BlockSize)
	} else {
		padLen = int(BlockSize + 56 - d.tc%BlockSize)
	}
	d.Write(tmp[0:padLen])
	var lenBuf [8]byte
	binary.LittleEndian.PutUint64(lenBuf[:], d.tc<<3)
	d.Write(lenBuf[:])
	if d.nx != 0 {
		panic("internal error: d.nx != 0")
	}
	var digestOut [Size]byte
	for i, s := range d.s {
		digestOut[i*4] = byte(s)
		digestOut[i*4+1] = byte(s >> 8)
		digestOut[i*4+2] = byte(s >> 16)
		digestOut[i*4+3] = byte(s >> 24)
	}
	return append(in, digestOut[:]...)
}

// rol performs a circular left rotation of x by n bits.
func rol(x uint32, n uint32) uint32 {
	return (x << n) | (x >> (32 - n))
}

// Nonlinear functions.
func f1(x, y, z uint32) uint32 { return x ^ y ^ z }
func f2(x, y, z uint32) uint32 { return (x & y) | (^x & z) }
func f3(x, y, z uint32) uint32 { return (x | ^y) ^ z }
func f4(x, y, z uint32) uint32 { return (x & z) | (y & ^z) }
func f5(x, y, z uint32) uint32 { return x ^ (y | ^z) }

// Left line round parameters.
var r1 = [16]uint32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
var s1 = [16]uint32{11, 14, 15, 12, 5, 8, 7, 9, 11, 13, 14, 15, 6, 7, 9, 8}

var r2 = [16]uint32{7, 4, 13, 1, 10, 6, 15, 3, 12, 0, 9, 5, 2, 14, 11, 8}
var s2 = [16]uint32{7, 6, 8, 13, 11, 9, 7, 15, 7, 12, 15, 9, 11, 7, 13, 12}

var r3 = [16]uint32{3, 10, 14, 4, 9, 15, 8, 1, 2, 7, 0, 6, 13, 11, 5, 12}
var s3 = [16]uint32{11, 13, 6, 7, 14, 9, 13, 15, 14, 8, 13, 6, 5, 12, 7, 5}

var r4 = [16]uint32{1, 9, 11, 10, 0, 8, 12, 4, 13, 3, 7, 15, 14, 5, 6, 2}
var s4 = [16]uint32{11, 12, 14, 15, 14, 15, 9, 8, 9, 14, 5, 6, 8, 6, 5, 12}

var r5 = [16]uint32{4, 0, 5, 9, 7, 12, 2, 10, 14, 1, 3, 8, 11, 6, 15, 13}
var s5 = [16]uint32{7, 5, 8, 11, 14, 14, 12, 6, 9, 13, 15, 7, 12, 8, 9, 11}

// Right line round parameters.
var rr1 = [16]uint32{5, 14, 7, 0, 9, 2, 11, 4, 13, 6, 15, 8, 1, 10, 3, 12}
var ss1 = [16]uint32{8, 9, 9, 11, 13, 15, 15, 5, 7, 7, 8, 11, 14, 14, 12, 6}

var rr2 = [16]uint32{6, 11, 3, 7, 0, 13, 5, 10, 14, 15, 8, 12, 4, 9, 1, 2}
var ss2 = [16]uint32{9, 13, 15, 7, 12, 8, 9, 11, 7, 7, 12, 7, 6, 15, 13, 11}

var rr3 = [16]uint32{15, 5, 1, 3, 7, 14, 6, 9, 11, 8, 12, 2, 10, 0, 4, 13}
var ss3 = [16]uint32{9, 7, 15, 11, 8, 6, 6, 14, 12, 13, 5, 14, 13, 13, 7, 5}

var rr4 = [16]uint32{8, 6, 4, 1, 3, 11, 15, 0, 5, 12, 2, 13, 9, 7, 10, 14}
var ss4 = [16]uint32{15, 5, 8, 11, 14, 14, 6, 14, 6, 9, 12, 9, 12, 5, 15, 8}

var rr5 = [16]uint32{12, 15, 10, 4, 1, 5, 8, 7, 6, 2, 13, 14, 0, 3, 9, 11}
var ss5 = [16]uint32{8, 5, 12, 9, 12, 5, 14, 6, 8, 13, 6, 5, 15, 13, 11, 11}

// block processes complete 64-byte chunks from p.
func block(d *digest, p []byte) int {
	n := len(p) / BlockSize
	if n == 0 {
		return 0
	}
	for i := 0; i < n; i++ {
		offset := i * BlockSize
		var X [16]uint32
		for j := 0; j < 16; j++ {
			X[j] = binary.LittleEndian.Uint32(p[offset+j*4 : offset+j*4+4])
		}
		// Save current state.
		A, B, C, D, E := d.s[0], d.s[1], d.s[2], d.s[3], d.s[4]
		A1, B1, C1, D1, E1 := A, B, C, D, E

		var T uint32
		// Left line rounds.
		for j := 0; j < 16; j++ {
			T = rol(A+f1(B, C, D)+X[r1[j]]+0, s1[j]) + E
			A, E, D, C, B = E, D, rol(C, 10), B, T
		}
		for j := 0; j < 16; j++ {
			T = rol(A+f2(B, C, D)+X[r2[j]]+0x5A827999, s2[j]) + E
			A, E, D, C, B = E, D, rol(C, 10), B, T
		}
		for j := 0; j < 16; j++ {
			T = rol(A+f3(B, C, D)+X[r3[j]]+0x6ED9EBA1, s3[j]) + E
			A, E, D, C, B = E, D, rol(C, 10), B, T
		}
		for j := 0; j < 16; j++ {
			T = rol(A+f4(B, C, D)+X[r4[j]]+0x8F1BBCDC, s4[j]) + E
			A, E, D, C, B = E, D, rol(C, 10), B, T
		}
		for j := 0; j < 16; j++ {
			T = rol(A+f5(B, C, D)+X[r5[j]]+0xA953FD4E, s5[j]) + E
			A, E, D, C, B = E, D, rol(C, 10), B, T
		}

		// Right line rounds.
		for j := 0; j < 16; j++ {
			T = rol(A1+f5(B1, C1, D1)+X[rr1[j]]+0x50A28BE6, ss1[j]) + E1
			A1, E1, D1, C1, B1 = E1, D1, rol(C1, 10), B1, T
		}
		for j := 0; j < 16; j++ {
			T = rol(A1+f4(B1, C1, D1)+X[rr2[j]]+0x5C4DD124, ss2[j]) + E1
			A1, E1, D1, C1, B1 = E1, D1, rol(C1, 10), B1, T
		}
		for j := 0; j < 16; j++ {
			T = rol(A1+f3(B1, C1, D1)+X[rr3[j]]+0x6D703EF3, ss3[j]) + E1
			A1, E1, D1, C1, B1 = E1, D1, rol(C1, 10), B1, T
		}
		for j := 0; j < 16; j++ {
			T = rol(A1+f2(B1, C1, D1)+X[rr4[j]]+0x7A6D76E9, ss4[j]) + E1
			A1, E1, D1, C1, B1 = E1, D1, rol(C1, 10), B1, T
		}
		for j := 0; j < 16; j++ {
			T = rol(A1+f1(B1, C1, D1)+X[rr5[j]]+0x00000000, ss5[j]) + E1
			A1, E1, D1, C1, B1 = E1, D1, rol(C1, 10), B1, T
		}

		T = d.s[1] + C + D1
		d.s[1] = d.s[2] + D + E1
		d.s[2] = d.s[3] + E + A1
		d.s[3] = d.s[4] + A + B1
		d.s[4] = d.s[0] + B + C1
		d.s[0] = T
	}
	return n * BlockSize
}
