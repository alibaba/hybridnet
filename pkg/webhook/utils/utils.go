package utils

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"strings"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/feature"
	"github.com/alibaba/hybridnet/pkg/utils"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SelectNetworkAndSubnetFromObject(ctx context.Context, c client.Reader, obj client.Object) (networkName string, subnetNameStr string, err error) {
	networkName = utils.PickFirstNonEmptyString(obj.GetAnnotations()[constants.AnnotationSpecifiedNetwork],
		obj.GetLabels()[constants.LabelSpecifiedNetwork])
	subnetNameStr = utils.PickFirstNonEmptyString(obj.GetAnnotations()[constants.AnnotationSpecifiedSubnet],
		obj.GetLabels()[constants.LabelSpecifiedSubnet])

	subnetNames := specifiedSubnetStrToSubnetNames(subnetNameStr)
	if len(subnetNames) > 2 {
		return "", "", fmt.Errorf("cannot have more than two specified subnet in dualstack")
	}

	var networkNameFromSubnet string
	for index, subnetName := range subnetNames {
		subnet := &networkingv1.Subnet{}
		if err = c.Get(ctx, types.NamespacedName{Name: subnetName}, subnet); err != nil {
			return "", "", fmt.Errorf("specified subnet %s not found", subnetName)
		}

		if len(subnetNames) == 2 {
			if index == 0 {
				if subnet.Spec.Range.Version != networkingv1.IPv4 {
					return "", "", fmt.Errorf("when both ipv4/ipv6 subnets are specified, " +
						"the subnet name in front of the \"/\" should be an ipv4 subnet")
				}
			} else {
				if subnet.Spec.Range.Version != networkingv1.IPv6 {
					return "", "", fmt.Errorf("when both ipv4/ipv6 subnets are specified, " +
						"the subnet name after the \"/\" should be an ipv6 subnet")
				}
			}
		}

		if len(networkNameFromSubnet) == 0 {
			networkNameFromSubnet = subnet.Spec.Network
		} else if networkNameFromSubnet != subnet.Spec.Network {
			return "", "", fmt.Errorf("the networks of ipv4/ipv6 subnets need to be the same")
		}
	}

	if len(networkNameFromSubnet) != 0 {
		if len(networkName) == 0 {
			// subnet can also determine the specified network
			networkName = networkNameFromSubnet
		}

		if networkName != networkNameFromSubnet {
			return "", "", fmt.Errorf("specified network and subnet conflict in %s %s/%s",
				obj.GetObjectKind().GroupVersionKind().String(),
				obj.GetNamespace(),
				obj.GetName(),
			)
		}
	}

	return
}

func specifiedSubnetStrToSubnetNames(specifiedSubnetString string) (subnetNames []string) {
	if len(specifiedSubnetString) > 0 {
		if feature.DualStackEnabled() {
			subnetNames = strings.Split(specifiedSubnetString, "/")
		} else {
			subnetNames = []string{specifiedSubnetString}
		}
	}
	return
}

func SubnetNameBelongsToSpecifiedSubnets(subnetName, specifiedSubnetString string) bool {
	subnetNames := specifiedSubnetStrToSubnetNames(specifiedSubnetString)
	for _, subnet := range subnetNames {
		if subnetName == subnet {
			return true
		}
	}
	return false
}

func AdmissionErroredWithLog(code int32, err error, logger logr.Logger) admission.Response {
	logger.Error(err, "admission error")
	return admission.Errored(code, err)
}

func AdmissionDeniedWithLog(reason string, logger logr.Logger) admission.Response {
	logger.Info("admission denied", "reason", reason)
	return admission.Denied(reason)
}
