module github.com/oecp/rama

go 1.13

require (
	github.com/containernetworking/cni v0.7.1
	github.com/containernetworking/plugins v0.8.3
	github.com/coreos/go-iptables v0.4.5
	github.com/elazarl/goproxy v0.0.0-20191011121108-aa519ddbe484 // indirect
	github.com/emicklei/go-restful v2.9.5+incompatible
	github.com/gogf/gf v1.16.4
	github.com/golang/groupcache v0.0.0-20191027212112-611e8accdfc9 // indirect
	github.com/heptiolabs/healthcheck v0.0.0-20180807145615-6ff867650f40
	github.com/json-iterator/go v1.1.10
	github.com/mdlayher/ethernet v0.0.0-20190606142754-0394541c37b7
	github.com/mdlayher/ndp v0.0.0-20200602162440-17ab9e3e5567
	github.com/mdlayher/raw v0.0.0-20190606142536-fef19f00fc18
	github.com/parnurzeal/gorequest v0.2.16
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.0.0
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/vishvananda/netlink v1.1.0
	github.com/vishvananda/netns v0.0.0-20191106174202-0a2b9b5464df
	go.uber.org/atomic v1.5.1 // indirect
	go.uber.org/multierr v1.4.0 // indirect
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b // indirect
	golang.org/x/oauth2 v0.0.0-20191202225959-858c2ad4c8b6 // indirect
	golang.org/x/sys v0.0.0-20200930185726-fdedc70b468f
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	gopkg.in/DATA-DOG/go-sqlmock.v1 v1.3.0 // indirect
	gopkg.in/errgo.v2 v2.1.0
	k8s.io/api v0.18.15
	k8s.io/apimachinery v0.20.4
	k8s.io/apiserver v0.20.4
	k8s.io/client-go v0.18.15
	k8s.io/component-base v0.20.4
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v0.0.0-00010101000000-000000000000
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	moul.io/http2curl v1.0.0 // indirect
	sigs.k8s.io/controller-runtime v0.6.5
)

replace k8s.io/kubernetes => k8s.io/kubernetes v1.18.15

replace (
	k8s.io/api => k8s.io/api v0.18.15
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.15
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.15
	k8s.io/apiserver => k8s.io/apiserver v0.18.15
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.18.15
	k8s.io/client-go => k8s.io/client-go v0.18.15
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.18.15
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.18.15
	k8s.io/code-generator => k8s.io/code-generator v0.18.15
	k8s.io/component-base => k8s.io/component-base v0.18.15
	k8s.io/cri-api => k8s.io/cri-api v0.18.15
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.18.15
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.18.15
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.18.15
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.18.15
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.18.15
	k8s.io/kubectl => k8s.io/kubectl v0.18.15
	k8s.io/kubelet => k8s.io/kubelet v0.18.15
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.18.15
	k8s.io/metrics => k8s.io/metrics v0.18.15
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.18.15
)

replace sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.6.5
