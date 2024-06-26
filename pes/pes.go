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

package pes

import "github.com/potterxu/gots/v2/packet"

// AlignedPUSI checks for a PUSI with aligned flag set and returns a bool
// indicating a match when true, as well as the bytes for the PES data
func AlignedPUSI(pkt *packet.Packet) ([]byte, bool) {
	if !pkt.PayloadUnitStartIndicator() {
	} else if pesHeaderBytes, err := packet.PESHeader(pkt); err != nil {
	} else if pesHeader, err := NewPESHeader(pesHeaderBytes); err != nil {
	} else if pesHeader != nil && pesHeader.DataAligned() {
		return pesHeader.Data(), true
	}
	return nil, false
}
