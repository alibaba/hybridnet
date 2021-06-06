package arping

// The MIT License (MIT)
//
// Copyright (c) 2014-2016 j-keck [jhyphenkeck@gmail.com]
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

import (
	"bytes"
	"encoding/binary"
	"net"
)

type Operation uint16

const (
	Request Operation = 1
	Reply   Operation = 2
)

type arpDatagram struct {
	htype uint16 // Hardware Type
	ptype uint16 // Protocol Type
	hlen  uint8  // Hardware address Length
	plen  uint8  // Protocol address length
	oper  uint16 // Operation 1->request, 2->response
	sha   []byte // Sender hardware address, length from Hlen
	spa   []byte // Sender protocol address, length from Plen
	tha   []byte // Target hardware address, length from Hlen
	tpa   []byte // Target protocol address, length from Plen
}

func newArpDatagram(
	op Operation,
	srcMac net.HardwareAddr,
	srcIP net.IP,
	dstMac net.HardwareAddr,
	dstIP net.IP) arpDatagram {
	return arpDatagram{
		htype: uint16(1),
		ptype: uint16(0x0800),
		hlen:  uint8(6),
		plen:  uint8(4),
		oper:  uint16(op),
		sha:   srcMac,
		spa:   srcIP.To4(),
		tha:   dstMac,
		tpa:   dstIP.To4()}
}

func (datagram arpDatagram) Marshal() []byte {
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, datagram.htype)
	_ = binary.Write(buf, binary.BigEndian, datagram.ptype)
	_ = binary.Write(buf, binary.BigEndian, datagram.hlen)
	_ = binary.Write(buf, binary.BigEndian, datagram.plen)
	_ = binary.Write(buf, binary.BigEndian, datagram.oper)
	buf.Write(datagram.sha)
	buf.Write(datagram.spa)
	buf.Write(datagram.tha)
	buf.Write(datagram.tpa)

	return buf.Bytes()
}

func (datagram arpDatagram) MarshalWithEthernetHeader() []byte {
	// ethernet frame header
	var ethernetHeader []byte
	ethernetHeader = append(ethernetHeader, datagram.tha...)
	ethernetHeader = append(ethernetHeader, datagram.sha...)
	ethernetHeader = append(ethernetHeader, []byte{0x08, 0x06}...) // arp

	return append(ethernetHeader, datagram.Marshal()...)
}

func (datagram arpDatagram) SenderIP() net.IP {
	return net.IP(datagram.spa)
}
func (datagram arpDatagram) SenderMac() net.HardwareAddr {
	return net.HardwareAddr(datagram.sha)
}

func (datagram arpDatagram) IsResponseOf(request arpDatagram) bool {
	return datagram.oper == uint16(Reply) && bytes.Equal(request.spa, datagram.tpa) &&
		bytes.Equal(request.tpa, datagram.spa)
}

func parseArpDatagram(buffer []byte) arpDatagram {
	var datagram arpDatagram

	b := bytes.NewBuffer(buffer)
	_ = binary.Read(b, binary.BigEndian, &datagram.htype)
	_ = binary.Read(b, binary.BigEndian, &datagram.ptype)
	_ = binary.Read(b, binary.BigEndian, &datagram.hlen)
	_ = binary.Read(b, binary.BigEndian, &datagram.plen)
	_ = binary.Read(b, binary.BigEndian, &datagram.oper)

	haLen := int(datagram.hlen)
	paLen := int(datagram.plen)
	datagram.sha = b.Next(haLen)
	datagram.spa = b.Next(paLen)
	datagram.tha = b.Next(haLen)
	datagram.tpa = b.Next(paLen)

	return datagram
}
