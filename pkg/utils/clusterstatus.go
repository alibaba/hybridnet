package utils

import (
	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TypeHealthCheck       networkingv1.ClusterConditionType = "HealthCheck"
	TypeBidirectionalConn networkingv1.ClusterConditionType = "BidirectionalConnection"
	TypeSameOverlayNetID  networkingv1.ClusterConditionType = "SameOverlayNetID"

	ReasonClusterReady        = "ClusterReady"
	ReasonHealthCheckReady    = "HealthCheckReady"
	ReasonClusterNotReachable = "ClusterNotReachable"
	ReasonHealthCheckNotReady = "HealthCheckNotReady"
	ReasonClusterReachable    = "ClusterReachable"
	ReasonDoubleConn          = "BothSetRemoteCluster"
	ReasonNotDoubleConn       = "RemoteNotSetRemoteCluster"
	ReasonSameOverlayNetID    = "SameOverlayNetIDReady"
	ReasonNotSameOverlayNetID = "SameOverlayNetIDNotReady"

	MsgClusterReady           = "All check pass"
	MsgHealthzNotOk           = "/healthz responded without ok"
	MsgHealthzOk              = "/healthz responded with ok"
	MsgBidirectionalConnOk    = "Both Clusters have created remote cluster"
	MsgBidirectionalConnNotOk = "Remote Clusters have not apply remote-cluster-cr about local cluster"
	MsgSameOverlayNetID       = "Both clusters have same overlay net id"
)

func NewClusterReady() networkingv1.ClusterCondition {
	cur := metav1.Now()
	return networkingv1.ClusterCondition{
		Type:               networkingv1.ClusterReady,
		Status:             corev1.ConditionTrue,
		LastProbeTime:      cur,
		LastTransitionTime: &cur,
		Reason:             StringPtr(ReasonClusterReady),
		Message:            StringPtr(MsgClusterReady),
	}
}

func NewHealthCheckReady() networkingv1.ClusterCondition {
	cur := metav1.Now()
	return networkingv1.ClusterCondition{
		Type:               TypeHealthCheck,
		Status:             corev1.ConditionTrue,
		LastProbeTime:      cur,
		LastTransitionTime: &cur,
		Reason:             StringPtr(ReasonHealthCheckReady),
		Message:            StringPtr(MsgHealthzOk),
	}
}

func NewBidirectionalConnReady() networkingv1.ClusterCondition {
	cur := metav1.Now()
	return networkingv1.ClusterCondition{
		Type:               TypeBidirectionalConn,
		Status:             corev1.ConditionTrue,
		LastProbeTime:      cur,
		LastTransitionTime: &cur,
		Reason:             StringPtr(ReasonDoubleConn),
		Message:            StringPtr(MsgBidirectionalConnOk),
	}
}

func NewBidirectionalConnNotReady(s string) networkingv1.ClusterCondition {
	cur := metav1.Now()
	return networkingv1.ClusterCondition{
		Type:               TypeBidirectionalConn,
		Status:             corev1.ConditionFalse,
		LastProbeTime:      cur,
		LastTransitionTime: &cur,
		Reason:             StringPtr(ReasonNotDoubleConn),
		Message:            StringPtr(s),
	}
}

func NewOverlayNetIDReady() networkingv1.ClusterCondition {
	cur := metav1.Now()
	return networkingv1.ClusterCondition{
		Type:               TypeSameOverlayNetID,
		Status:             corev1.ConditionTrue,
		LastProbeTime:      cur,
		LastTransitionTime: &cur,
		Reason:             StringPtr(ReasonSameOverlayNetID),
		Message:            StringPtr(MsgSameOverlayNetID),
	}
}

func NewOverlayNetIDNotReady(s string) networkingv1.ClusterCondition {
	cur := metav1.Now()
	return networkingv1.ClusterCondition{
		Type:               TypeSameOverlayNetID,
		Status:             corev1.ConditionFalse,
		LastProbeTime:      cur,
		LastTransitionTime: &cur,
		Reason:             StringPtr(ReasonNotSameOverlayNetID),
		Message:            StringPtr(s),
	}
}

func NewClusterOffline(err error) networkingv1.ClusterCondition {
	cur := metav1.Now()
	return networkingv1.ClusterCondition{
		Type:               networkingv1.ClusterOffline,
		Status:             corev1.ConditionTrue,
		LastProbeTime:      cur,
		LastTransitionTime: &cur,
		Reason:             StringPtr(ReasonClusterNotReachable),
		Message:            StringPtr(err.Error()),
	}
}

func NewHealthCheckNotReady(err error) networkingv1.ClusterCondition {
	cur := metav1.Now()
	return networkingv1.ClusterCondition{
		Type:               TypeHealthCheck,
		Status:             corev1.ConditionFalse,
		LastProbeTime:      cur,
		LastTransitionTime: &cur,
		Reason:             StringPtr(ReasonHealthCheckNotReady),
		Message:            StringPtr(err.Error()),
	}
}

func NewClusterNotOffline() networkingv1.ClusterCondition {
	cur := metav1.Now()
	return networkingv1.ClusterCondition{
		Type:               networkingv1.ClusterOffline,
		Status:             corev1.ConditionFalse,
		LastProbeTime:      cur,
		LastTransitionTime: &cur,
		Reason:             StringPtr(ReasonClusterReachable),
		Message:            StringPtr(MsgHealthzNotOk),
	}
}

func StringPtr(s string) *string {
	return &s
}
