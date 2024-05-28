/*
MIT License

Copyright 2016 Comcast Cable Communications Management, LLC

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package packet

import (
	"encoding/hex"
	"testing"

	"github.com/potterxu/gots/v2"
)

// PacketAccumulator is not thread safe
// For a PSI specific example, see psi.PmtAccumulatorDoneFunc(PacketAccumulator)
func TestPacketAccumulator(t *testing.T) {
	b, _ := hex.DecodeString("474064100002b0ba0001c10000e065f00b0504435545490e03c03dd01be065f016970028046400283fe907108302808502800e03c0392087e066f0219700050445414333cc03c0c2100a04656e6700e907108302808502800e03c000f087e067f0219700050445414333cc03c0c4100a0473706100e907108302808502800e03c001e00fe068f01697000a04656e6700e907108302808502800e03c000f00fe069f01697000a0473706100e907108302808502800e03c000f086e0dc")
	firstPacket := &Packet{}
	copy(firstPacket[:], b)
	b, _ = hex.DecodeString("47006411f0002b59bc22ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	secondPacket := &Packet{}
	copy(secondPacket[:], b)

	var called = false

	packets := []*Packet{firstPacket, secondPacket}
	// Just a simple func to accumulate two packets
	dFunc := func(b []byte) (bool, error) {
		if len(b) <= PacketSize {
			return false, nil
		}

		called = true

		return true, nil
	}

	acc := NewAccumulator(dFunc)
	for _, pkt := range packets {
		_, err := acc.WritePacket(pkt)
		if err == gots.ErrAccumulatorDone {
			// Accumulation is done
			break
		} else if err != nil {
			t.Errorf("Unexpected accumulator error: %s", err)
		}
	}

	if !called {
		t.Error("Expected Accumulator doneFunc to be called")
	}
}
