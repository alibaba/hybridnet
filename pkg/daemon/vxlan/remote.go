package vxlan

import "net"

func (dev *Device) RecordRemoteVtepInfo(remoteVtepMac net.HardwareAddr, remoteVtepIP net.IP) {
	dev.remoteIPToMacMap[remoteVtepIP.String()] = remoteVtepMac
}
