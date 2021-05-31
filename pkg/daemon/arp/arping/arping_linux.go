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
	"net"
	"syscall"
)

func initialize(iface *net.Interface) (int, syscall.SockaddrLinklayer, error) {
	toSockaddr := syscall.SockaddrLinklayer{Ifindex: iface.Index}

	// 1544 = htons(ETH_P_ARP)
	const proto = 1544
	sock, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, proto)
	return sock, toSockaddr, err
}

func send(sock int, request arpDatagram, toSockaddr syscall.SockaddrLinklayer) error {
	return syscall.Sendto(sock, request.MarshalWithEthernetHeader(), 0, &toSockaddr)
}

func receive(sock int) (arpDatagram, error) {
	buffer := make([]byte, 128)
	n, _, err := syscall.Recvfrom(sock, buffer, 0)
	if err != nil {
		return arpDatagram{}, err
	}
	// skip 14 bytes ethernet header
	return parseArpDatagram(buffer[14:n]), nil
}

func deinitialize(sock int) error {
	return syscall.Close(sock)
}
