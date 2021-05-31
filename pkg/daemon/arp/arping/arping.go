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
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/mdlayher/ethernet"
)

var (
	// ErrTimeout error
	ErrTimeout = errors.New("timeout")
)

// PingOverIface sends an arp ping over interface 'iface' to 'dstIP'
func PingOverIface(srcIP, dstIP net.IP, iface *net.Interface, timeout time.Duration) (net.HardwareAddr, error) {
	if err := validateIP(dstIP); err != nil {
		return nil, err
	}

	srcMac := iface.HardwareAddr
	request := newArpDatagram(Request, srcMac, srcIP, ethernet.Broadcast, dstIP)

	sock, toSockaddr, err := initialize(iface)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = deinitialize(sock)
	}()

	type PingResult struct {
		mac net.HardwareAddr
		err error
	}
	pingResultChan := make(chan PingResult, 1)

	// send arp request once
	if err := send(sock, request, toSockaddr); err != nil {
		return nil, fmt.Errorf("send arp request over interface %v failed: %v", iface.Name, err)
	}

	go func() {
		for {
			// receive arp response
			response, err := receive(sock)

			if err != nil {
				pingResultChan <- PingResult{nil, err}
				return
			}

			if response.IsResponseOf(request) {
				pingResultChan <- PingResult{response.SenderMac(), err}
				return
			}
		}
	}()

	select {
	case pingResult := <-pingResultChan:
		return pingResult.mac, pingResult.err
	case <-time.After(timeout):
		return nil, ErrTimeout
	}
}

func validateIP(ip net.IP) error {
	// ip must be a valid V4 address
	if len(ip.To4()) != net.IPv4len {
		return fmt.Errorf("not a valid v4 Address: %s", ip)
	}
	return nil
}

func GratuitousOverIface(ip net.IP, iface *net.Interface) error {
	sock, toSockaddr, err := initialize(iface)
	if err != nil {
		return err
	}

	defer func() {
		_ = deinitialize(sock)
	}()

	for _, op := range []Operation{Request, Reply} {
		pkt := newArpDatagram(op, iface.HardwareAddr, ip, ethernet.Broadcast, ip)

		// send arp request once
		if err := send(sock, pkt, toSockaddr); err != nil {
			return err
		}
	}

	return nil
}
