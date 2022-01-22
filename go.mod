module github.com/alibaba/hybridnet

go 1.15

require (
	github.com/containernetworking/cni v0.8.1
	github.com/containernetworking/plugins v0.0.0-00010101000000-000000000000
	github.com/coreos/go-iptables v0.6.0
	github.com/emicklei/go-restful v2.15.0+incompatible
	github.com/go-logr/logr v0.3.0
	github.com/gogf/gf v1.16.6
	github.com/heptiolabs/healthcheck v0.0.0-20211123025425-613501dd5deb
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mdlayher/ethernet v0.0.0-20190606142754-0394541c37b7
	github.com/mdlayher/ndp v0.0.0-20200602162440-17ab9e3e5567
	github.com/mdlayher/raw v0.0.0-20190606142536-fef19f00fc18
	github.com/mikioh/ipaddr v0.0.0-20190404000644-d465c8ab6721
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.17.0
	github.com/osrg/gobgp/v3 v3.0.0-rc4
	github.com/parnurzeal/gorequest v0.2.16
	github.com/prometheus/client_golang v1.11.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/vishvananda/netlink v1.1.1-0.20210330154013-f5de75959ad5
	github.com/vishvananda/netns v0.0.0-20211101163701-50045581ed74
	golang.org/x/sys v0.0.0-20211205182925-97ca703d548d
	google.golang.org/protobuf v1.27.1
	gopkg.in/DATA-DOG/go-sqlmock.v1 v1.3.0 // indirect
	k8s.io/api v0.20.13
	k8s.io/apimachinery v0.20.13
	k8s.io/apiserver v0.20.13
	k8s.io/client-go v0.20.13
	k8s.io/component-base v0.20.13
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v0.0.0-00010101000000-000000000000
	k8s.io/utils v0.0.0-20211208161948-7d6a63dca704
	moul.io/http2curl v1.0.0 // indirect
	sigs.k8s.io/controller-runtime v0.0.0-00010101000000-000000000000
)

replace k8s.io/kubernetes => k8s.io/kubernetes v1.20.13

replace (
	k8s.io/api => k8s.io/api v0.20.13
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.13
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.13
	k8s.io/apiserver => k8s.io/apiserver v0.20.13
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.20.13
	k8s.io/client-go => k8s.io/client-go v0.20.13
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.20.13
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.20.13
	k8s.io/code-generator => k8s.io/code-generator v0.20.13
	k8s.io/component-base => k8s.io/component-base v0.20.13
	k8s.io/component-helpers => k8s.io/component-helpers v0.20.13
	k8s.io/controller-manager => k8s.io/controller-manager v0.20.13
	k8s.io/cri-api => k8s.io/cri-api v0.20.13
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.20.13
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.20.13
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.20.13
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.20.13
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.20.13
	k8s.io/kubectl => k8s.io/kubectl v0.20.13
	k8s.io/kubelet => k8s.io/kubelet v0.20.13
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.20.13
	k8s.io/metrics => k8s.io/metrics v0.20.13
	k8s.io/mount-utils => k8s.io/mount-utils v0.20.13
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.20.13
)

replace sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.8.3

replace github.com/containernetworking/cni => github.com/containernetworking/cni v0.8.1

replace github.com/containernetworking/plugins => github.com/containernetworking/plugins v0.9.1
