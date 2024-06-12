# Image prefetcher

This is a utility for quickly fetching OCI images onto Kubernetes cluster nodes.

Talks directly to Container Runtime Interface ([CRI](https://kubernetes.io/docs/concepts/architecture/cri/)) API to:
- fetch all images on all nodes in parallel,
- retry pulls with increasingly longer timeouts. This prevents getting stuck on stalled connections to image registry.

It also optionally collects each pull attempt's duration and result.

## Architecture

### `image-prefetcher`

- main binary,
- shipped as an OCI image,
- provides three subcommands:
  - `fetch`: runs the actual image pulls via CRI, meant to run as an init container
    of DaemonSet pods.
    Requires access to the CRI UNIX domain socket from the host.
  - `sleep`: just sleeps forever, meant to run as the main container of DaemonSet pods.
  - `aggregate-metrics`: runs a gRPC server which collects data points pushed by the
    `fetch` pods, and makes the data available for download over HTTP.
    Meant to run as a standalone pod.

### `deploy`

- a helper command-line utility for generating `image-prefetcher` manifests,
- separate go module, with no dependencies outside Go standard library.

## Usage

1. First, run the `deploy` binary to generate a manifest for an instance of `image-prefetcher`.

   You can run many instances independently.

   It requires a single positional argument for the **name** of the instance.
   This also determines the name of a `ConfigMap` supplying names of images to fetch.

   It also accepts a few optional flags:
   - `--version`: `image-prefetcher` OCI image tag. See [list of existing tags](https://quay.io/repository/mowsiany/image-prefetcher?tab=tags).
   - `--k8s-flavor` depending on the cluster. Currently one of:
     - `vanilla`: a generic Kubernetes distribution without additional restrictions.
     - `ocp`: OpenShift, which requires explicitly granting special privileges.
   - `--secret`: image pull `Secret` name. Required if the images are not pullable anonymously.
     This image pull secret should be usable for all images fetched by the given instance.
     If provided, it must be of type `kubernetes.io/dockerconfigjson` and exist in the same namespace.
   - `--collect-metrics`: if the image pull metrics should be collected.

   Example:

   ```
   go run github.com/stackrox/image-prefetcher/deploy@v0.2.2 my-images v0.2.2 vanilla > manifest.yaml
   ```

2. Prepare an image list. This should be a plain text file with one image name per line.
   Lines starting with `#` and blank ones are ignored.
   ```
   echo debian:latest >> image-list.txt
   echo quay.io/strimzi/kafka:latest-kafka-3.7.0 >> image-list.txt
   ```

3. Deploy:
   ```
   kubectl create namespace prefetch-images
   kubectl create -n prefetch-images configmap my-images --from-file="images.txt=image-list.txt"
   kubectl apply -n prefetch-images -f manifest.yaml
   ```

4. Wait for the pull to complete, with a timeout:
   ```
   kubectl rollout -n prefetch-images status daemonset my-images --timeout 5m
   ```

5. If something goes wrong, look at logs:
   ```
   kubectl logs -n prefetch-images daemonset/my-images -c prefetch
   ```

6. If metrics collection was requested, wait for the endpoint to appear, and fetch them:
   ```
   attempt=0
   service="service/my-images-metrics"
   while [[ -z $(kubectl -n "${ns}" get "${service}" -o jsonpath="{.status.loadBalancer.ingress}" 2>/dev/null) ]]; do
       if [ "$attempt" -lt "10" ]; then
           echo "Waiting for ${service} to obtain endpoint ..."
           ((attempt++))
           sleep 10
       else
           echo "Timeout waiting for ${service} to obtain endpoint!"
           exit 1
       fi
   done
   endpoint="$(kubectl -n "${ns}" get "${service}" -o json | jq -r '.status.loadBalancer.ingress[] | .ip')"
   curl "http://${endpoint}:8080/metrics" | jq
   ```
   
   See the [Result](internal/metrics/metrics.proto) message definition for a list of fields.

### Customization

You can tweak certain parameters such as timeouts by editing `args` in the above manifest.
See the [fetch command](./cmd/fetch.go) for accepted flags.

## Limitations

This utility was designed for small, ephemeral test clusters, in order to improve reliability and speed of end-to-end tests.

If deployed on larger clusters, it may have a "thundering herd" effect on the OCI registries it pulls from.
This is because all images are pulled from all nodes in parallel.

## Release procedure

1. Pick a tag name, use the usual semver rules. We'll refer to it as `vx.y.z` below
2. [Draft a new release](https://github.com/stackrox/image-prefetcher/releases/new)
   1. Enter `vx.y.z` as the name of a new tag to create
   2. Click "Create new tag on publish"
   3. Keep `master` as target
   4. Keep `auto` as previous tag
   5. Click "Generate release notes"
   6. Optional: edit the release notes as you see fit
3. Publish the release
4. Make sure the build GitHub Action that gets triggered by the tag runs successfully and pushes images.
5. It is also a good idea to wait for the e2e job to pass before proceeding.
6. Create a tag for the `deploy` module
   1. This is the tag that `go run github.com/stackrox/image-prefetcher/deploy@vx.y.z` looks for (since its `go.mod` is
      not in the repository root)
   2. Currently, this needs to be done manually since GitHub UI does not seem to allow creation of tags without
      an associated release. TODO: [automate this](https://github.com/stackrox/image-prefetcher/issues/30)
   3. Check out the tagged commit in your clone
   4. `git tag deploy/vx.y.z`
   5. `git push --tags`
