binary:
	CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' .
