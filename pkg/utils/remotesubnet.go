package utils

import (
	"fmt"
)

const remoteSubnetNameFormat = "%v.%v"

type RemoteSubnetName string

func GenRemoteSubnetName(clusterName string, subnetName string) string {
	return fmt.Sprintf(remoteSubnetNameFormat, clusterName, subnetName)
}
