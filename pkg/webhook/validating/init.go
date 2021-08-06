package validating

import (
	clientset "github.com/oecp/rama/pkg/client/clientset/versioned"
	ramainformer "github.com/oecp/rama/pkg/client/informers/externalversions"
	v1 "github.com/oecp/rama/pkg/client/listers/networking/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const UserAgent = "rama-webhook"

var (
	RCLister v1.RemoteClusterLister
)

func InitRemoteClusterInformer(stopCh <-chan struct{}) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	ramaClient, err := clientset.NewForConfig(rest.AddUserAgent(config, UserAgent))
	ramaInformerFactory := ramainformer.NewSharedInformerFactory(ramaClient, 0)
	RCLister = ramaInformerFactory.Networking().V1().RemoteClusters().Lister()
	rcSynced := ramaInformerFactory.Networking().V1().RemoteClusters().Informer().HasSynced

	go ramaInformerFactory.Start(stopCh)
	if ok := cache.WaitForCacheSync(stopCh, rcSynced); !ok {
		panic("failed to wait for caches to sync")
	}
}
