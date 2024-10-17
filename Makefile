binary-amd64:
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o image-prefetcher-amd64 -ldflags '-extldflags "-static"' .

binary-arm64:
	env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -o image-prefetcher-arm64 -ldflags '-extldflags "-static"' .

binary-ppc64le:
	env CGO_ENABLED=0 GOOS=linux GOARCH=ppc64le go build -a -o image-prefetcher-ppc64le -ldflags '-extldflags "-static"' .

binary-s390x:
	env CGO_ENABLED=0 GOOS=linux GOARCH=s390x go build -a -o image-prefetcher-s390x -ldflags '-extldflags "-static"' .

binary-all: binary-amd64 binary-arm64 binary-ppc64le binary-s390x

push-multi-arch-manifest:
	docker manifest create '${IMAGE_TAG}' \
	--amend '${IMAGE_TAG}-amd64' \
	--amend	'${IMAGE_TAG}-arm64' \
	--amend	'${IMAGE_TAG}-ppc64le' \
	--amend	'${IMAGE_TAG}-s390x'
	docker manifest push '${IMAGE_TAG}'
