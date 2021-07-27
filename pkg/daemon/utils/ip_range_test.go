package utils

import (
	"fmt"
	"net"
	"testing"
)

type TestSubnetSpec struct {
	cidr           *net.IPNet
	includedRanges []*IPRange
	gateway        net.IP
	excludeIPs     []net.IP
	reservedIPs    []net.IP

	expectIPBlocks []*net.IPNet
}

func TestFindSubnetExcludeIPBlocks(t *testing.T) {
	testCases := []TestSubnetSpec{
		{
			cidr: &net.IPNet{
				IP:   net.ParseIP("192.168.3.0"),
				Mask: net.CIDRMask(24, 32),
			},
			expectIPBlocks: nil,
		}, {
			cidr: &net.IPNet{
				IP:   net.ParseIP("192.168.3.0"),
				Mask: net.CIDRMask(24, 32),
			},
			gateway: net.ParseIP("192.168.3.1"),
			expectIPBlocks: []*net.IPNet{
				{
					IP:   net.ParseIP("192.168.3.1"),
					Mask: net.CIDRMask(32, 32),
				},
			},
		}, {
			cidr: &net.IPNet{
				IP:   net.ParseIP("192.168.3.0"),
				Mask: net.CIDRMask(24, 32),
			},
			gateway: net.ParseIP("192.168.3.1"),
			includedRanges: []*IPRange{
				{
					start: net.ParseIP("192.168.3.100"),
					end:   net.ParseIP("192.168.3.200"),
				},
			},
			expectIPBlocks: []*net.IPNet{
				{
					IP:   net.ParseIP("192.168.3.0"),
					Mask: net.CIDRMask(26, 32),
				}, {
					IP:   net.ParseIP("192.168.3.64"),
					Mask: net.CIDRMask(27, 32),
				}, {
					IP:   net.ParseIP("192.168.3.96"),
					Mask: net.CIDRMask(30, 32),
				}, {
					IP:   net.ParseIP("192.168.3.201"),
					Mask: net.CIDRMask(32, 32),
				}, {
					IP:   net.ParseIP("192.168.3.202"),
					Mask: net.CIDRMask(31, 32),
				}, {
					IP:   net.ParseIP("192.168.3.204"),
					Mask: net.CIDRMask(30, 32),
				}, {
					IP:   net.ParseIP("192.168.3.208"),
					Mask: net.CIDRMask(28, 32),
				}, {
					IP:   net.ParseIP("192.168.3.224"),
					Mask: net.CIDRMask(27, 32),
				},
			},
		}, {
			cidr: &net.IPNet{
				IP:   net.ParseIP("192.168.3.0"),
				Mask: net.CIDRMask(24, 32),
			},
			gateway: net.ParseIP("192.168.3.1"),
			includedRanges: []*IPRange{
				{
					start: net.ParseIP("192.168.3.100"),
					end:   net.ParseIP("192.168.3.150"),
				}, {
					start: net.ParseIP("192.168.3.151"),
					end:   net.ParseIP("192.168.3.200"),
				},
			},
			expectIPBlocks: []*net.IPNet{
				{
					IP:   net.ParseIP("192.168.3.0"),
					Mask: net.CIDRMask(26, 32),
				}, {
					IP:   net.ParseIP("192.168.3.64"),
					Mask: net.CIDRMask(27, 32),
				}, {
					IP:   net.ParseIP("192.168.3.96"),
					Mask: net.CIDRMask(30, 32),
				}, {
					IP:   net.ParseIP("192.168.3.201"),
					Mask: net.CIDRMask(32, 32),
				}, {
					IP:   net.ParseIP("192.168.3.202"),
					Mask: net.CIDRMask(31, 32),
				}, {
					IP:   net.ParseIP("192.168.3.204"),
					Mask: net.CIDRMask(30, 32),
				}, {
					IP:   net.ParseIP("192.168.3.208"),
					Mask: net.CIDRMask(28, 32),
				}, {
					IP:   net.ParseIP("192.168.3.224"),
					Mask: net.CIDRMask(27, 32),
				},
			},
		}, {
			cidr: &net.IPNet{
				IP:   net.ParseIP("192.168.3.0"),
				Mask: net.CIDRMask(24, 32),
			},
			gateway: net.ParseIP("192.168.3.1"),
			includedRanges: []*IPRange{
				{
					start: net.ParseIP("192.168.3.100"),
					end:   net.ParseIP("192.168.3.150"),
				}, {
					start: net.ParseIP("192.168.3.152"),
					end:   net.ParseIP("192.168.3.200"),
				}, {
					start: net.ParseIP("192.168.3.208"),
					end:   net.ParseIP("192.168.3.223"),
				},
			},
			expectIPBlocks: []*net.IPNet{
				{
					IP:   net.ParseIP("192.168.3.0"),
					Mask: net.CIDRMask(26, 32),
				}, {
					IP:   net.ParseIP("192.168.3.64"),
					Mask: net.CIDRMask(27, 32),
				}, {
					IP:   net.ParseIP("192.168.3.96"),
					Mask: net.CIDRMask(30, 32),
				}, {
					IP:   net.ParseIP("192.168.3.201"),
					Mask: net.CIDRMask(32, 32),
				}, {
					IP:   net.ParseIP("192.168.3.202"),
					Mask: net.CIDRMask(31, 32),
				}, {
					IP:   net.ParseIP("192.168.3.204"),
					Mask: net.CIDRMask(30, 32),
				}, {
					IP:   net.ParseIP("192.168.3.151"),
					Mask: net.CIDRMask(32, 32),
				}, {
					IP:   net.ParseIP("192.168.3.224"),
					Mask: net.CIDRMask(27, 32),
				},
			},
		}, {
			cidr: &net.IPNet{
				IP:   net.ParseIP("192.168.3.0"),
				Mask: net.CIDRMask(24, 32),
			},
			gateway: net.ParseIP("192.168.3.1"),
			includedRanges: []*IPRange{
				{
					start: net.ParseIP("192.168.3.100"),
					end:   net.ParseIP("192.168.3.150"),
				}, {
					start: net.ParseIP("192.168.3.151"),
					end:   net.ParseIP("192.168.3.200"),
				}, {
					start: net.ParseIP("192.168.3.208"),
					end:   net.ParseIP("192.168.3.223"),
				},
			},
			excludeIPs: []net.IP{
				net.ParseIP("192.168.3.50"),
				net.ParseIP("192.168.3.120"),
				net.ParseIP("192.168.3.121"),
			},
			reservedIPs: []net.IP{
				net.ParseIP("192.168.3.160"),
				net.ParseIP("192.168.3.207"),
				net.ParseIP("192.168.3.224"),
			},
			expectIPBlocks: []*net.IPNet{
				{
					IP:   net.ParseIP("192.168.3.0"),
					Mask: net.CIDRMask(26, 32),
				}, {
					IP:   net.ParseIP("192.168.3.64"),
					Mask: net.CIDRMask(27, 32),
				}, {
					IP:   net.ParseIP("192.168.3.96"),
					Mask: net.CIDRMask(30, 32),
				}, {
					IP:   net.ParseIP("192.168.3.120"),
					Mask: net.CIDRMask(31, 32),
				}, {
					IP:   net.ParseIP("192.168.3.160"),
					Mask: net.CIDRMask(32, 32),
				}, {
					IP:   net.ParseIP("192.168.3.201"),
					Mask: net.CIDRMask(32, 32),
				}, {
					IP:   net.ParseIP("192.168.3.202"),
					Mask: net.CIDRMask(31, 32),
				}, {
					IP:   net.ParseIP("192.168.3.204"),
					Mask: net.CIDRMask(30, 32),
				}, {
					IP:   net.ParseIP("192.168.3.224"),
					Mask: net.CIDRMask(27, 32),
				},
			},
		}, {
			cidr: &net.IPNet{
				IP:   net.ParseIP("192.168.3.100"),
				Mask: net.CIDRMask(32, 32),
			},
			excludeIPs: []net.IP{
				net.ParseIP("192.168.3.100"),
			},
			reservedIPs: []net.IP{
				net.ParseIP("192.168.3.100"),
			},
			expectIPBlocks: []*net.IPNet{
				{
					IP:   net.ParseIP("192.168.3.100"),
					Mask: net.CIDRMask(32, 32),
				},
			},
		}, {
			cidr: &net.IPNet{
				IP:   net.ParseIP("192.168.3.100"),
				Mask: net.CIDRMask(32, 32),
			},
			reservedIPs: []net.IP{
				net.ParseIP("192.168.3.100"),
			},
			expectIPBlocks: []*net.IPNet{
				{
					IP:   net.ParseIP("192.168.3.100"),
					Mask: net.CIDRMask(32, 32),
				},
			},
		},
	}

	for _, test := range testCases {
		ipBlocks, _ := FindSubnetExcludeIPBlocks(test.cidr, test.includedRanges,
			test.gateway, test.excludeIPs, test.reservedIPs)

		if !blockSliceEqual(ipBlocks, test.expectIPBlocks) {
			t.Fatalf("failed to parse ip range %v, result ip blocks: %v", test.String(), ipBlocks)
		}
	}
}

func (ts *TestSubnetSpec) String() string {
	return fmt.Sprintf("cidr: %v, includedIPRanges: %v, gateway: %v, excludeIPs: %v, reservedIPs: %v",
		ts.cidr.String(), ts.includedRanges, ts.gateway.String(), ts.excludeIPs, ts.reservedIPs)
}

func blockSliceEqual(slice1, slice2 []*net.IPNet) bool {
	if len(slice1) != len(slice2) {
		return false
	}

	for _, block := range slice1 {
		found := false
		for _, targetBlock := range slice2 {
			if targetBlock.String() == block.String() {
				found = true
				break
			}
		}

		if !found {
			return false
		}
	}

	return true
}
