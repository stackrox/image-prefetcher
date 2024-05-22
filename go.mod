module github.com/stackrox/image-prefetcher

go 1.22.0

toolchain go1.22.1

require (
	github.com/cenkalti/backoff/v4 v4.2.1
	github.com/google/uuid v1.6.0
	github.com/spf13/cobra v1.8.0
	github.com/spf13/pflag v1.0.5
	google.golang.org/grpc v1.63.2
	google.golang.org/protobuf v1.33.0
	k8s.io/apimachinery v0.30.0
	k8s.io/cri-api v0.29.3
	k8s.io/klog/v2 v2.120.1
)

require (
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	golang.org/x/net v0.23.0 // indirect
	golang.org/x/sys v0.18.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240227224415-6ceb2ff114de // indirect
)
