/*
 Copyright 2021 The Hybridnet Authors.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package vxlan

import (
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"

	daemonutils "github.com/alibaba/hybridnet/pkg/daemon/utils"

	"github.com/vishvananda/netlink"
)

var (
	broadcastFdbMac, _ = net.ParseMAC("00:00:00:00:00:00")
)

type Device struct {
	link *netlink.Vxlan

	// remote vtep ip and mac address it should be forward to.
	remoteIPToMacMap map[string]net.HardwareAddr
}

func NewVxlanDevice(name string, vxlanID int, parent string, localAddr net.IP, port int, baseReachableTime time.Duration,
	learning bool) (*Device, error) {
	parentLink, err := netlink.LinkByName(parent)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent link %v: %v", parent, err)
	}

	link := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: name,

			// Use parent's mac as hardware address.
			HardwareAddr: parentLink.Attrs().HardwareAddr,
		},
		VxlanId:      vxlanID,
		VtepDevIndex: parentLink.Attrs().Index,
		SrcAddr:      localAddr,
		Port:         port,
		Learning:     learning,
	}

	link, err = ensureLink(link)
	if err != nil {
		return nil, err
	}

	sysctlPath := fmt.Sprintf(containernetwork.IPv4AppSolicitSysctl, link.Name)
	if err := daemonutils.SetSysctlIgnoreNotExist(sysctlPath, 1); err != nil {
		return nil, fmt.Errorf("failed to set sysctl parameter %v: %v", sysctlPath, err)
	}

	sysctlPath = fmt.Sprintf(containernetwork.IPv4BaseReachableTimeMSSysctl, link.Name)
	if err := daemonutils.SetSysctlIgnoreNotExist(sysctlPath, int(1000*baseReachableTime.Seconds())); err != nil {
		return nil, fmt.Errorf("failed to set sysctl parameter %v: %v", sysctlPath, err)
	}

	ipv6Disabled, err := containernetwork.CheckIPv6Disabled(link.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check ipv6 disables for link %v: %v", link.Name, err)
	}

	if !ipv6Disabled {
		sysctlPath = fmt.Sprintf(containernetwork.IPv6AppSolicitSysctl, link.Name)
		if err := daemonutils.SetSysctlIgnoreNotExist(sysctlPath, 1); err != nil {
			return nil, fmt.Errorf("failed to set sysctl parameter %v: %v", sysctlPath, err)
		}

		sysctlPath = fmt.Sprintf(containernetwork.IPv6BaseReachableTimeMSSysctl, link.Name)
		if err := daemonutils.SetSysctlIgnoreNotExist(sysctlPath, int(1000*baseReachableTime.Seconds())); err != nil {
			return nil, fmt.Errorf("failed to set sysctl parameter %v: %v", sysctlPath, err)
		}

		sysctlPath = fmt.Sprintf(containernetwork.AcceptRASysctl, link.Name)
		if err := daemonutils.SetSysctl(sysctlPath, 0); err != nil {
			return nil, fmt.Errorf("failed to set sysctl parameter %v: %v", sysctlPath, err)
		}
	}

	return &Device{
		link:             link,
		remoteIPToMacMap: map[string]net.HardwareAddr{},
	}, nil
}

func (dev *Device) MacAddr() net.HardwareAddr {
	return dev.link.HardwareAddr
}

func (dev *Device) Link() *netlink.Vxlan {
	return dev.link
}

func (dev *Device) RecordVtepInfo(vtepMac net.HardwareAddr, vtepIP net.IP) {
	dev.remoteIPToMacMap[vtepIP.String()] = vtepMac
}

func (dev *Device) SyncVtepInfo() error {
	for remoteIPString, macAddr := range dev.remoteIPToMacMap {
		unicastFdbEntry := netlink.Neigh{
			LinkIndex:    dev.link.Index,
			Family:       syscall.AF_BRIDGE,
			State:        netlink.NUD_PERMANENT,
			Flags:        netlink.NTF_SELF,
			IP:           net.ParseIP(remoteIPString),
			HardwareAddr: macAddr,
		}

		// Duplicate append action will not case error.
		if err := netlink.NeighAppend(&unicastFdbEntry); err != nil {
			return fmt.Errorf("failed to append unicast fdb entry %v for interface %v: %v", unicastFdbEntry.String(), dev.link.Name, err)
		}

		broadcastFdbEntry := netlink.Neigh{
			LinkIndex:    dev.link.Index,
			Family:       syscall.AF_BRIDGE,
			State:        netlink.NUD_PERMANENT,
			Flags:        netlink.NTF_SELF,
			IP:           net.ParseIP(remoteIPString),
			HardwareAddr: broadcastFdbMac,
		}

		// Duplicate append action will not case error.
		if err := netlink.NeighAppend(&broadcastFdbEntry); err != nil {
			return fmt.Errorf("failed to append broadcast fdb entry %v for interface %v: %v", broadcastFdbEntry.String(), dev.link.Name, err)
		}
	}

	fdbEntryList, err := netlink.NeighList(dev.link.Attrs().Index, syscall.AF_BRIDGE)
	if err != nil {
		return fmt.Errorf("failed to list neigh: %v", err)
	}

	for _, entry := range fdbEntryList {
		// Delete invalid entries.
		if vtepMac, exist := dev.remoteIPToMacMap[entry.IP.String()]; !exist ||
			(vtepMac.String() != entry.HardwareAddr.String() &&
				entry.HardwareAddr.String() != broadcastFdbMac.String() && entry.HardwareAddr != nil) {
			entry.Family = syscall.AF_BRIDGE
			if err := netlink.NeighDel(&entry); err != nil {
				return fmt.Errorf("failed to delete fdb entry %v for interface %v: %v", entry.String(), dev.link.Name, err)
			}
		}
	}

	return nil
}

func ensureLink(vxlan *netlink.Vxlan) (*netlink.Vxlan, error) {
	err := netlink.LinkAdd(vxlan)
	if err == syscall.EEXIST {
		// it's ok if the device already exists as long as config is similar
		existing, err := netlink.LinkByName(vxlan.Name)
		if err != nil {
			return nil, err
		}

		incompat := vxlanLinksIncompat(vxlan, existing)
		if incompat == "" {
			if err := netlink.LinkSetUp(existing); err != nil {
				return nil, fmt.Errorf("failed to set link %v up: %v", existing.Attrs().Name, err)
			}

			return existing.(*netlink.Vxlan), nil
		}

		// delete existing
		if err = netlink.LinkDel(existing); err != nil {
			return nil, fmt.Errorf("failed to delete interface: %v", err)
		}

		// create new
		if err = netlink.LinkAdd(vxlan); err != nil {
			return nil, fmt.Errorf("failed to create vxlan interface: %v", err)
		}
	} else if err != nil {
		return nil, err
	}

	ifIndex := vxlan.Index
	link, err := netlink.LinkByIndex(vxlan.Index)
	if err != nil {
		return nil, fmt.Errorf("can't locate created vxlan device with index %v", ifIndex)
	}

	var ok bool
	if vxlan, ok = link.(*netlink.Vxlan); !ok {
		return nil, fmt.Errorf("created vxlan device with index %v is not vxlan", ifIndex)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return nil, fmt.Errorf("failed to set link %v up: %v", link.Attrs().Name, err)
	}

	return vxlan, nil
}

func vxlanLinksIncompat(l1, l2 netlink.Link) string {
	if l1.Type() != l2.Type() {
		return fmt.Sprintf("link type: %v vs %v", l1.Type(), l2.Type())
	}

	v1, ok := l1.(*netlink.Vxlan)
	if !ok {
		return fmt.Sprintf("link %v is not vxlan device", l1.Attrs().Name)
	}

	v2, ok := l2.(*netlink.Vxlan)
	if !ok {
		return fmt.Sprintf("link %v is not vxlan device", l2.Attrs().Name)
	}

	if v1.VxlanId != v2.VxlanId {
		return fmt.Sprintf("vni: %v vs %v", v1.VxlanId, v2.VxlanId)
	}

	if v1.VtepDevIndex > 0 && v2.VtepDevIndex > 0 && v1.VtepDevIndex != v2.VtepDevIndex {
		return fmt.Sprintf("vtep (external) interface: %v vs %v", v1.VtepDevIndex, v2.VtepDevIndex)
	}

	if v1.HardwareAddr.String() != v2.HardwareAddr.String() {
		return fmt.Sprintf("vtep Mac: %v vs %v", v1.HardwareAddr, v2.HardwareAddr)
	}

	if len(v1.SrcAddr) > 0 && len(v2.SrcAddr) > 0 && !v1.SrcAddr.Equal(v2.SrcAddr) {
		return fmt.Sprintf("vtep (external) IP: %v vs %v", v1.SrcAddr, v2.SrcAddr)
	}

	if len(v1.Group) > 0 && len(v2.Group) > 0 && !v1.Group.Equal(v2.Group) {
		return fmt.Sprintf("group address: %v vs %v", v1.Group, v2.Group)
	}

	if v1.L2miss != v2.L2miss {
		return fmt.Sprintf("l2miss: %v vs %v", v1.L2miss, v2.L2miss)
	}

	if v1.Port > 0 && v2.Port > 0 && v1.Port != v2.Port {
		return fmt.Sprintf("port: %v vs %v", v1.Port, v2.Port)
	}

	if v1.GBP != v2.GBP {
		return fmt.Sprintf("gbp: %v vs %v", v1.GBP, v2.GBP)
	}

	return ""
}
