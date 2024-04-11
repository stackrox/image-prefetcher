# Image prefetcher

This is a utility for quickly fetching OCI images onto Kubernetes cluster nodes.

Talks directly to Container Runtime Interface ([CRI](https://kubernetes.io/docs/concepts/architecture/cri/)) API to:
- fetch all images on all nodes in parallel,
- retry pulls with increasingly longer timeouts. This prevents getting stuck on stalled connections to image registry.

## Architecture

### `image-prefetcher`

- main binary,
- meant to be run in pods of a DaemonSet,
- shipped as an OCI image,
- provides two subcommands:
  - `fetch`: runs the actual image pulls via CRI, meant to run as an init container,
    Requires access to the CRI UNIX domain socket from the host.
  - `sleep`: just sleeps forever, meant to run as the main container,

### `deploy`

- a helper command-line utility for generating `image-prefetcher` manifests,
- separate go module, with no dependencies outside Go standard library.

## Usage

1. First, run the `deploy` binary to generate a manifest for an instance of `image-prefetcher`.

   You can run many instances independently.

   It requires a few arguments:
   - **name** of the instance.
     This also determines the name of a `ConfigMap` supplying names of images to fetch.
   - `image-prefetcher` OCI image **version**. See [list of existing tags](https://quay.io/repository/mowsiany/image-prefetcher?tab=tags).
   - **cluster flavor**. Currently one of:
     - `vanilla`: a generic Kubernetes distribution without additional restrictions.
     - `ocp`: OpenShift, which requires explicitly granting special privileges.
   - optional **image pull `Secret` name**. Required if the images are not pullable anonymously.
     This image pull secret should be usable for all images fetched by the given instance.
     If provided, it must be of type `kubernetes.io/dockerconfigjson` and exist in the same namespace.

   Example:

   ```
   go run github.com/stackrox/image-prefetcher/deploy@main my-images v0.0.8 vanilla > manifest.yaml
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

### Customization

You can tweak certain parameters such as timeouts by editing `args` in the above manifest.
See the [fetch command](./cmd/fetch.go) for accepted flags.

## Limitations

This utility was designed for small, ephemeral test clusters, in order to improve reliability and speed of end-to-end tests.

If deployed on larger clusters, it may have a "thundering herd" effect on the OCI registries it pulls from.
This is because all images are pulled from all nodes in parallel.
