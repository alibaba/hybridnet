package utils

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/feature"
	"github.com/alibaba/hybridnet/pkg/ipam/strategy"
	"github.com/alibaba/hybridnet/pkg/utils"
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
		subnetNames = strings.Split(specifiedSubnetString, "/")
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

// ParseNetworkConfigOfPodByPriority will try to parse network-related configs for pod by priority as below,
// 1. if pod was stateful allocated and no need to be reallocated, reusing the existing network
// 2. if pod have labels or annotations which contain network config, use it all
// 3. if namespace which pod locates on have labels or annotations which contain network config, use it all
func ParseNetworkConfigOfPodByPriority(ctx context.Context, c client.Reader, pod *corev1.Pod) (networkNameStr, subnetNameStr, networkTypeStr, ipFamilyStr string, err error) {
	var (
		// elected will be true iff one networking config was assigned
		elected = func() bool {
			return len(networkNameStr) > 0 || len(subnetNameStr) > 0 || len(networkTypeStr) > 0 || len(ipFamilyStr) > 0
		}

		// fetchFromObject will fetch networking configs from k8s objects
		fetchFromObject = func(obj client.Object) error {
			if networkNameStr, subnetNameStr, err = SelectNetworkAndSubnetFromObject(ctx, c, obj); err != nil {
				return fmt.Errorf("unable to select network and subnet from object %s/%s/%s: %v",
					obj.GetObjectKind().GroupVersionKind().String(), obj.GetNamespace(), obj.GetName(), err)
			}
			networkTypeStr = utils.PickFirstNonEmptyString(obj.GetAnnotations()[constants.AnnotationNetworkType],
				obj.GetLabels()[constants.LabelNetworkType])
			ipFamilyStr = obj.GetAnnotations()[constants.AnnotationIPFamily]
			return nil
		}
	)

	// priority 1
	if strategy.OwnByStatefulWorkload(pod) {
		var shouldReuse = utils.ParseBoolOrDefault(pod.Annotations[constants.AnnotationIPRetain], strategy.DefaultIPRetain)
		if shouldReuse {
			ipList := &networkingv1.IPInstanceList{}
			if err = c.List(
				ctx,
				ipList,
				client.InNamespace(pod.Namespace),
				client.MatchingLabels{
					constants.LabelPod: pod.Name,
				}); err != nil {
				return
			}

			// ignore terminating ipInstance
			for i := range ipList.Items {
				if ipList.Items[i].DeletionTimestamp == nil {
					networkNameStr = ipList.Items[i].Spec.Network
					break
				}
			}
		}
	} else if feature.VMIPRetainEnabled() {
		var isVMPod bool
		var vmName string
		isVMPod, vmName, _, err = strategy.OwnByVirtualMachine(ctx, pod, c)
		if err != nil {
			err = fmt.Errorf("unable to check if pod %v/%v is for VM: %v", pod.Namespace, pod.Name, err)
			return
		}

		if isVMPod {
			ipList := &networkingv1.IPInstanceList{}
			if err = c.List(
				ctx,
				ipList,
				client.InNamespace(pod.Namespace),
				client.MatchingLabels{
					constants.LabelVM: vmName,
				}); err != nil {
				err = fmt.Errorf("failed to list allocated ip instances for vm %v: %v", vmName, err)
				return
			}

			// ignore terminating ipInstance
			for i := range ipList.Items {
				if ipList.Items[i].DeletionTimestamp == nil {
					networkNameStr = ipList.Items[i].Spec.Network
					break
				}
			}
		}
	}

	// priority level 2
	if !elected() {
		if err = fetchFromObject(pod); err != nil {
			return
		}
	}

	// priority level 3
	if !elected() {
		ns := &corev1.Namespace{}
		if err = c.Get(ctx, types.NamespacedName{Name: pod.Namespace}, ns); err != nil {
			return
		}
		if err = fetchFromObject(ns); err != nil {
			return
		}
	}

	return
}
