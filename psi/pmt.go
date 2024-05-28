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

package psi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/potterxu/gots/v2"
	"github.com/potterxu/gots/v2/packet"
)

const PidNotFound int = 1<<16 - 1

const (
	programInfoLengthOffset         = 10 // includes PSIHeaderLen
	pmtEsDescriptorStaticLen uint16 = 5
)

// Unaccounted bytes before the end of the SectionLength field
const (
	// Pointerfield(1) + table id(1) + flags(.5) + section length (2.5)
	PSIHeaderLen uint16 = 4
	CrcLen       uint16 = 4
)

// PMT is a Program Map Table.
type PMT interface {
	Pids() []int
	VersionNumber() uint8
	CurrentNextIndicator() bool
	ElementaryStreams() []PmtElementaryStream
	RemoveElementaryStreams(pids []int)
	IsPidForStreamWherePresentationLagsEbp(pid int) bool
	String() string
	PIDExists(pid int) bool
}

type pmt struct {
	pids                 []int
	elementaryStreams    []PmtElementaryStream
	versionNumber        uint8
	currentNextIndicator bool
}

// PmtAccumulatorDoneFunc is a doneFunc that can be used for packet accumulation
// to create a PMT
func PmtAccumulatorDoneFunc(b []byte) (bool, error) {
	if len(b) < 1 {
		return false, nil
	}

	start := 1 + int(PointerField(b))
	if len(b) < start {
		return false, nil
	}

	sectionBytes := b[start:]
	for len(sectionBytes) > 2 && sectionBytes[0] != 0xFF {
		tableLength := sectionLength(sectionBytes)
		if len(sectionBytes) < int(tableLength)+3 {
			return false, nil
		}
		sectionBytes = sectionBytes[3+tableLength:]
	}

	return true, nil
}

// NewPMT Creates a new PMT from the given bytes.
// pmtBytes should be concatenated packet payload contents.
func NewPMT(pmtBytes []byte) (PMT, error) {
	pmt := &pmt{}
	err := pmt.parseTables(pmtBytes)
	if err != nil {
		return nil, err
	}
	return pmt, nil
}

func (p *pmt) parseTables(pmtBytes []byte) error {
	sectionBytes := pmtBytes[1+PointerField(pmtBytes):]

	for len(sectionBytes) > 2 && sectionBytes[0] != 0xFF {
		tableLength := sectionLength(sectionBytes)

		if tableID(sectionBytes) == 0x2 {
			err := p.parsePMTSection(sectionBytes[0 : 3+tableLength])
			if err != nil {
				return err
			}
		}
		sectionBytes = sectionBytes[3+tableLength:]
	}

	return nil
}

func (p *pmt) parsePMTSection(pmtBytes []byte) error {
	var pids []int
	var elementaryStreams []PmtElementaryStream
	sectionLength := sectionLength(pmtBytes)

	if len(pmtBytes) <= programInfoLengthOffset+1 {
		return gots.ErrPMTParse
	}

	var err error
	p.versionNumber, p.currentNextIndicator, err = tableVersionAndCNI(pmtBytes)
	if err != nil {
		return err
	}

	programInfoLength := uint16(pmtBytes[programInfoLengthOffset]&0x0f)<<8 |
		uint16(pmtBytes[programInfoLengthOffset+1])

	// start at the stream descriptors, parse until the CRC
	for offset := programInfoLengthOffset + 2 + programInfoLength; offset < PSIHeaderLen+sectionLength-pmtEsDescriptorStaticLen-CrcLen; {
		elementaryStreamType := uint8(pmtBytes[offset])
		elementaryPid := int(pmtBytes[offset+1]&0x1f)<<8 | int(pmtBytes[offset+2])
		pids = append(pids, elementaryPid)
		infoLength := uint16(pmtBytes[offset+3]&0x0f)<<8 | uint16(pmtBytes[offset+4])

		// Move past the es descriptor static data
		offset += pmtEsDescriptorStaticLen
		var descriptors []PmtDescriptor
		if infoLength != 0 && int(infoLength+offset) < len(pmtBytes) {
			var descriptorOffset uint16
			for descriptorOffset < infoLength {
				tag := uint8(pmtBytes[offset+descriptorOffset])
				descriptorOffset++
				descriptorLength := uint16(pmtBytes[offset+descriptorOffset])
				descriptorOffset++
				startPos := offset + descriptorOffset
				endPos := int(offset + descriptorOffset + descriptorLength)
				if endPos < len(pmtBytes) {
					data := pmtBytes[startPos:endPos]
					descriptors = append(descriptors, NewPmtDescriptor(tag, data))
				} else {
					return gots.ErrParsePMTDescriptor
				}
				descriptorOffset += descriptorLength
			}
			offset += infoLength
		}
		es := NewPmtElementaryStream(elementaryStreamType, elementaryPid, descriptors)
		elementaryStreams = append(elementaryStreams, es)
	}

	p.pids = pids
	p.elementaryStreams = elementaryStreams
	return nil
}

// Pids returns a slice of Pids
func (p *pmt) Pids() []int {
	return p.pids
}

// VersionNumber returns the version number of the PMT
func (p *pmt) VersionNumber() uint8 {
	return p.versionNumber
}

// CurrentNextIndicator provides a bool for if this PMT is in use yet
func (p *pmt) CurrentNextIndicator() bool {
	return p.currentNextIndicator
}

// ElementaryStreams returns a slice of PMT Elementary Streams
func (p *pmt) ElementaryStreams() []PmtElementaryStream {
	return p.elementaryStreams
}

// RemoveElementaryStreams removes elementary streams in the pmt of the given pids
func (p *pmt) RemoveElementaryStreams(removePids []int) {
	for _, pid := range removePids {
		for j, s := range p.elementaryStreams {
			if pid == s.ElementaryPid() {
				p.elementaryStreams = append(p.elementaryStreams[:j], p.elementaryStreams[j+1:]...)
				break
			}
		}
	}

	var filteredPids []int

	for _, es := range p.elementaryStreams {
		filteredPids = append(filteredPids, es.ElementaryPid())
	}

	p.pids = filteredPids
}

func (p *pmt) IsPidForStreamWherePresentationLagsEbp(pid int) bool {
	for _, s := range p.elementaryStreams {
		if pid == s.ElementaryPid() {
			return s.IsStreamWherePresentationLagsEbp()
		}
	}
	return false
}

func (p *pmt) String() string {
	var buf bytes.Buffer
	buf.WriteString("PMT[")
	i := 0
	for _, es := range p.elementaryStreams {
		buf.WriteString(fmt.Sprintf("%v", es))
		i++
		if i < len(p.elementaryStreams) {
			buf.WriteString(",")
		}
	}
	buf.WriteString("]")

	return buf.String()
}

func (p *pmt) PIDExists(pid int) bool {
	for _, pmtPid := range p.Pids() {
		if pmtPid == pid {
			return true
		}
	}
	return false
}

func ExtractCRC(payload []byte) (uint32, error) {
	if len(payload) < 4 {
		return 0, gots.ErrShortPayload
	}

	sectionLength := SectionLength(payload)

	if !CanBuildPMT(payload, sectionLength) {
		return 0, gots.ErrPMTParse
	}

	end := PSIHeaderLen + sectionLength

	// The CRC is the last 4-bytes of the PSI Table.
	data := payload[end-4 : end]
	return binary.BigEndian.Uint32(data), nil
}

func CanBuildPMT(payload []byte, sectionLength uint16) bool {
	if len(payload) < int(sectionLength) {
		return false
	}
	return true
}

// FilterPMTPacketsToPids filters the PMT contents of the provided packet to the PIDs provided and returns a new packet(s).
// For example: if the provided PMT has PIDs 101, 102, and 103 and the provided PIDs are 101 and 102,
//
//	the new PMT will have only descriptors for PID 101 and 102. The descriptor for PID 103 will be stripped from the new PMT packet.
//
// Returns packets and nil error if all pids are present in the PMT.
// Returns packets and non-nil error if some pids are present in the PMT.
// Returns nil packets and non-nil error if none of the pids are present in the PMT.
func FilterPMTPacketsToPids(packets []*packet.Packet, pids []int) ([]*packet.Packet, error) {
	// make sure we have packets
	if len(packets) == 0 {
		return nil, nil
	}

	if len(pids) == 0 {
		return packets, nil
	}

	// Mush the payloads of all PMT packets into one []byte
	var pmtByteBuffer bytes.Buffer
	for i := 0; i < len(packets); i++ {
		pay, err := packet.Payload(packets[i])
		if err != nil {
			return nil, gots.ErrNoPayload
		}
		pmtByteBuffer.Write(pay)
	}

	pmtPayload := pmtByteBuffer.Bytes()

	// Determine if any of the given PIDs aren't in the PMT.
	unfilteredPMT, _ := NewPMT(pmtPayload)

	pmtPid := packet.Pid(packets[0])
	var missingPids []int
	for _, pid := range pids {
		// Ignore PAT and PMT PIDS if they are included.
		if !unfilteredPMT.PIDExists(pid) && pid != PatPid && pid != pmtPid {
			missingPids = append(missingPids, pid)
		}
	}

	// Return an error if any of the given PIDs is not present in the PMT.
	var returnError error
	if len(missingPids) > 0 {
		returnError = fmt.Errorf(gots.ErrPIDNotInPMT.Error(), missingPids)
	}

	// Return nil packets and an error if none of the PIDs being filtered exist in the PMT.
	if len(missingPids) == len(pids) {
		return nil, returnError
	}

	// include +1 to account for the PointerField field itself
	pointerField := PointerField(pmtPayload) + 1

	var filteredPMT bytes.Buffer

	// Copy everything from the pointerfield offset and move the pmtPayload slice to the start of that
	filteredPMT.Write(pmtPayload[:pointerField])
	pmtPayload = pmtPayload[pointerField:]
	// Copy the first 12 bytes of the PMT packet. Only section_length will change.
	filteredPMT.Write(pmtPayload[:programInfoLengthOffset+2])

	// Get the section length
	sectionLength := sectionLength(pmtPayload)

	// Get program info length
	programInfoLength := uint16(pmtPayload[programInfoLengthOffset]&0x0f)<<8 | uint16(pmtPayload[programInfoLengthOffset+1])
	if programInfoLength != 0 {
		filteredPMT.Write(pmtPayload[programInfoLengthOffset+2 : programInfoLengthOffset+2+programInfoLength])
	}

	for offset := programInfoLengthOffset + 2 + programInfoLength; offset < PSIHeaderLen+sectionLength-pmtEsDescriptorStaticLen-CrcLen; {
		elementaryPid := int(pmtPayload[offset+1]&0x1f)<<8 | int(pmtPayload[offset+2])
		infoLength := uint16(pmtPayload[offset+3]&0x0f)<<8 | uint16(pmtPayload[offset+4])

		// This is an ES PID we want to keep
		if pidIn(pids, elementaryPid) {
			// write out the whole es info
			filteredPMT.Write(pmtPayload[offset : offset+pmtEsDescriptorStaticLen+infoLength])
		}
		offset += pmtEsDescriptorStaticLen + infoLength
	}

	// Create the new section length
	fPMT := filteredPMT.Bytes()
	// section_length is the length of data (including the CRC) in bytes following the section length field (ISO13818: 2.4.4.9)
	// This will be the length of our buffer - (Bytes preceding section_length) + CRC
	// Bytes preceding = 4 + PointerField value and the CRC = 4, so it turns out to be the length of the buffer - PointerField field
	// -1 because we previously added 1 for the pointerfield field itself
	newSectionLength := uint16(len(fPMT)) - uint16(pointerField-1)
	sectionLengthBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(sectionLengthBytes, newSectionLength)
	fPMT[pointerField+1] = (fPMT[pointerField+1] & 0xf0) | sectionLengthBytes[0]
	fPMT[pointerField+2] = sectionLengthBytes[1]

	// Recalculate the CRC
	fPMT = append(fPMT, gots.ComputeCRC(fPMT[pointerField:])...)

	var filteredPMTPackets []*packet.Packet
	for _, pkt := range packets {
		var pktBuf bytes.Buffer
		header := packet.Header(pkt)
		pktBuf.Write(header)
		if len(fPMT) > 0 {
			toWrite := safeSlice(fPMT, 0, packet.PacketSize-len(header))
			// truncate fPMT to the remaining bytes
			if len(toWrite) < len(fPMT) {
				fPMT = fPMT[len(toWrite):]
			} else {
				fPMT = nil
			}
			pktBuf.Write(toWrite)
		} else {
			// all done
			break
		}
		filteredPMTPackets = append(filteredPMTPackets, padPacket(&pktBuf))
	}
	return filteredPMTPackets, returnError
}

// IsPMT returns true if the provided packet is a PMT
// defined by the PAT provided. Returns ErrNilPAT if pat
// is nil, or any error encountered in parsing the PID
// of pkt.
func IsPMT(pkt *packet.Packet, pat PAT) (bool, error) {
	if pat == nil {
		return false, gots.ErrNilPAT
	}

	pmtMap := pat.ProgramMap()
	pid := packet.Pid(pkt)

	for _, mapPID := range pmtMap {
		if pid == mapPID {
			return true, nil
		}
	}

	return false, nil
}

func safeSlice(byteArray []byte, start, end int) []byte {
	if end < len(byteArray) {
		return byteArray[start:end]
	}
	return byteArray[start:len(byteArray)]
}

func padPacket(buf *bytes.Buffer) *packet.Packet {
	var pkt packet.Packet
	for i := copy(pkt[:], buf.Bytes()); i < packet.PacketSize; i++ {
		pkt[i] = 0xff
	}
	return &pkt
}

func pidIn(pids []int, target int) bool {
	for _, pid := range pids {
		if pid == target {
			return true
		}
	}

	return false
}

// ReadPMT extracts a PMT from a reader of a TS stream. It will read until PMT
// packet(s) are found or EOF is reached.
// It returns a new PMT object parsed from the packet(s), if found, and
// otherwise returns an error.
func ReadPMT(r io.Reader, pid int) (PMT, error) {
	var pkt = &packet.Packet{}
	var err error
	var pmt PMT

	pmtAcc := packet.NewAccumulator(PmtAccumulatorDoneFunc)
	done := false

	for !done {
		if _, err := io.ReadFull(r, pkt[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil, gots.ErrPMTNotFound
			}
			return nil, err
		}
		currPid := pkt.PID()
		if currPid != pid {
			continue
		}
		_, err = pmtAcc.WritePacket(pkt)
		if err == gots.ErrAccumulatorDone {
			pmt, err = NewPMT(pmtAcc.Bytes())
			if err != nil {
				return nil, err
			}
			if len(pmt.Pids()) == 0 {
				done = false
				pmtAcc = packet.NewAccumulator(PmtAccumulatorDoneFunc)
				continue
			}
			done = true
		} else if err != nil {
			return nil, err
		}

	}
	return pmt, nil
}
