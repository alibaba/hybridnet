package utils

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const remoteVtepNameFormat = "%v.%v"

func GenRemoteVtepName(clusterName string, nodeName string) string {
	if nodeName == "" || clusterName == "" {
		return ""
	}
	return fmt.Sprintf(remoteVtepNameFormat, clusterName, nodeName)
}

func NewRemoteVtep(clusterName string, uid types.UID, vtepIP, macAddr, nodeName string, endpointIPList []string) *networkingv1.RemoteVtep {
	vtep := &networkingv1.RemoteVtep{
		ObjectMeta: metav1.ObjectMeta{
			Name: GenRemoteVtepName(clusterName, nodeName),
			Labels: map[string]string{
				constants.LabelCluster: clusterName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: networkingv1.SchemeGroupVersion.String(),
					Kind:       "RemoteCluster",
					Name:       clusterName,
					UID:        uid,
					Controller: pointer.BoolPtr(true),
				},
			},
		},
		Spec: networkingv1.RemoteVtepSpec{
			ClusterName:    clusterName,
			NodeName:       nodeName,
			VtepIP:         vtepIP,
			VtepMAC:        macAddr,
			EndpointIPList: endpointIPList,
		},
		Status: networkingv1.RemoteVtepStatus{
			LastModifyTime: metav1.Now(),
		},
	}
	return vtep
}
