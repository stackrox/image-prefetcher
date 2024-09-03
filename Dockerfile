FROM golang:1.22.0 AS build
WORKDIR /build
COPY ./ ./
RUN go mod verify
RUN CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' .

FROM scratch
COPY --from=build /build/image-prefetcher /image-prefetcher
ENTRYPOINT ["/image-prefetcher"]
