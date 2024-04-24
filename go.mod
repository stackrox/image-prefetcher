module github.com/stackrox/image-prefetcher

go 1.21

toolchain go1.21.7

require (
	github.com/spf13/cobra v1.8.0
	google.golang.org/grpc v1.63.2
	k8s.io/cri-api v0.29.3
	k8s.io/kubernetes v1.29.3
)

// This set of replaces is needed to use k8s.io/kubernetes
// See https://github.com/kubernetes/kubernetes/issues/79384#issuecomment-505627280
// for more background.
// TODO(porridge): upgrade to 1.30.x and document the process
replace (
	k8s.io/cli-runtime => k8s.io/cli-runtime v1.29.3
	k8s.io/cloud-provider => k8s.io/cloud-provider v1.29.3
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v1.29.3
	k8s.io/component-helpers => k8s.io/component-helpers v1.29.3
	k8s.io/controller-manager => k8s.io/controller-manager v1.29.3
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v1.29.3
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v1.29.3
	k8s.io/endpointslice => k8s.io/endpointslice v1.29.3
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v1.29.3
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v1.29.3
	k8s.io/kube-proxy => k8s.io/kube-proxy v1.29.3
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v1.29.3
	k8s.io/kubectl => k8s.io/kubectl v1.29.3
	k8s.io/kubelet => k8s.io/kubelet v1.29.3
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v1.29.3
	k8s.io/metrics => k8s.io/metrics v1.29.3
	k8s.io/mount-utils => k8s.io/mount-utils v1.29.3
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v1.29.3
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v1.29.3
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/prometheus/client_golang v1.16.0 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.10.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/net v0.21.0 // indirect
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240227224415-6ceb2ff114de // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/apiextensions-apiserver v0.29.3 // indirect
	k8s.io/apimachinery v0.29.3 // indirect
	k8s.io/apiserver v0.29.3 // indirect
	k8s.io/component-base v0.29.3 // indirect
	k8s.io/klog/v2 v2.110.1 // indirect
)
