package utils

import "fmt"

type SubnetCidrTracker struct {
	subnetClusterMap map[string]string
	conflict         error
}

func NewSubnetCidrTracker() *SubnetCidrTracker {
	return &SubnetCidrTracker{map[string]string{}, nil}
}

func (tracker *SubnetCidrTracker) Refresh() {
	tracker.subnetClusterMap = map[string]string{}
	tracker.conflict = nil
}

func (tracker *SubnetCidrTracker) Track(cidr, cluster string) error {
	if lastCluster, exists := tracker.subnetClusterMap[cidr]; !exists || cluster == lastCluster {
		tracker.subnetClusterMap[cidr] = cluster
	} else {
		tracker.conflict = fmt.Errorf("cluster %s and cluster %s have a conflict in subnet config (cidr=%s)", lastCluster, cluster, cidr)
	}

	return tracker.conflict
}

func (tracker *SubnetCidrTracker) Where(cidr string) string {
	return tracker.subnetClusterMap[cidr]
}

func (tracker *SubnetCidrTracker) Conflict() error {
	return tracker.conflict
}
