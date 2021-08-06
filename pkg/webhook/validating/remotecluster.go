package validating

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sync"

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/utils"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	r                = regexp.MustCompile(`^(https?://)[\w-]+(\.[\w-]+)+:\d{1,5}$`)
	validateLock     sync.Mutex
	remoteClusterGVK = gvkConverter(ramav1.SchemeGroupVersion.WithKind("RemoteCluster"))
)

func init() {
	createHandlers[remoteClusterGVK] = RCCreateValidation
	updateHandlers[remoteClusterGVK] = RCUpdateValidation
	deleteHandlers[remoteClusterGVK] = RCDeleteValidation
}

func RCCreateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	rc := &ramav1.RemoteCluster{}
	err := handler.Decoder.Decode(*req, rc)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	return validate(rc)
}

func RCUpdateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	var (
		err   error
		newRC *ramav1.RemoteCluster
	)
	if err = handler.Decoder.DecodeRaw(req.Object, newRC); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	return validate(newRC)
}

func RCDeleteValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	return admission.Allowed("validation pass")
}

func validate(rc *ramav1.RemoteCluster) admission.Response {
	validateLock.Lock()
	defer validateLock.Unlock()

	connConfig := rc.Spec.ConnConfig
	if connConfig.Endpoint == "" || connConfig.CABundle == nil || connConfig.ClientKey == nil || connConfig.ClientCert == nil {
		return admission.Denied("empty connection config, please check.")
	}
	if !r.Match([]byte(connConfig.Endpoint)) {
		return admission.Denied("endpoint format: https://server:address, please check")
	}

	cfg, err := utils.BuildClusterConfig(rc)
	if err != nil {
		return admission.Denied(fmt.Sprintf("Can't build connection to remote cluster, maybe wrong config. Err=%v", err.Error()))
	}
	client := kubernetes.NewForConfigOrDie(cfg)
	uuid, err := utils.GetUUID(client)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	rcs, err := RCLister.List(labels.NewSelector())
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	for _, rc := range rcs {
		if uuid == rc.Status.UUID {
			return admission.Denied("Duplicate cluster configuration")
		}
	}
	return admission.Allowed("validation pass")
}
