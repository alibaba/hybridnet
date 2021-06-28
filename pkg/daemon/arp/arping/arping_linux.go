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
	"fmt"
	"net"
	"syscall"
	"time"
)

type LinuxSocket struct {
	sock       int
	toSockaddr syscall.SockaddrLinklayer
}

func initialize(iface net.Interface) (s *LinuxSocket, err error) {
	s = &LinuxSocket{}
	s.toSockaddr = syscall.SockaddrLinklayer{Ifindex: iface.Index}

	// 1544 = htons(ETH_P_ARP)
	const proto = 1544
	s.sock, err = syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, proto)
	return s, err
}

func (s *LinuxSocket) send(request arpDatagram) (time.Time, error) {
	return time.Now(), syscall.Sendto(s.sock, request.MarshalWithEthernetHeader(), 0, &s.toSockaddr)
}

func (s *LinuxSocket) receive() (arpDatagram, time.Time, error) {
	buffer := make([]byte, 128)
	n, _, err := syscall.Recvfrom(s.sock, buffer, 0)
	if err != nil {
		return arpDatagram{}, time.Now(), err
	}
	if n <= 14 {
		// amount of bytes read by socket is less than an ethernet header. clearly not what we look for
		return arpDatagram{}, time.Now(), fmt.Errorf("buffer with invalid length")

	}
	// skip 14 bytes ethernet header
	return parseArpDatagram(buffer[14:n]), time.Now(), nil
}

func (s *LinuxSocket) deinitialize() error {
	return syscall.Close(s.sock)
}
