# Node Labeling

The image prefetcher automatically labels nodes based on the overall success or failure of the prefetch operation. This allows deployments to use label selectors to schedule pods only on nodes where all images have been successfully prefetched.

## Label Format

The prefetcher creates a single label per instance on each node:

- **Label key**: `image-prefetcher.stackrox.io/<instance-name>`
- **Label value**:
  - `succeeded` - if ALL images were successfully pulled
  - `failed` - if ANY image failed to pull

The instance name in the label key is the name you provide when deploying (e.g., `my-images`). Multiple independent prefetcher instances can run simultaneously, each creating its own label.

## Example Label

After successfully prefetching all images with instance name `my-images`, a node will have:

```
image-prefetcher.stackrox.io/my-images=succeeded
```

If any image fails:

```
image-prefetcher.stackrox.io/my-images=failed
```

## Using Label Selectors in Deployments

You can use node selectors or node affinity to schedule pods only on nodes where all images have been successfully prefetched:

**Example 1: Simple Node Selector**
```yaml
spec:
  nodeSelector:
    image-prefetcher.stackrox.io/my-images: succeeded
  containers:
  - name: my-app
    image: nginx:latest
```

**Example 2: Node Affinity**
```yaml
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: image-prefetcher.stackrox.io/my-images
            operator: In
            values: [succeeded]
  containers:
  - name: my-app
    image: nginx:latest
```

**Example 3: Finding Nodes with Successful Prefetch**
```bash
kubectl get nodes -l 'image-prefetcher.stackrox.io/my-images=succeeded'
kubectl get nodes -l 'image-prefetcher.stackrox.io'
```

## Label Lifecycle

- Each prefetcher instance updates only its own label when it runs.
- Other instances' labels are left untouched, allowing multiple independent prefetchers to coexist.
- This enables running different prefetchers for different image sets on the same nodes.
- The strict all-or-nothing approach ensures you only schedule on nodes where the entire image set is available.

## RBAC Requirements

The node labeling feature requires `get`, `patch`, and `update` permissions on `nodes` resources. A `ServiceAccount`, `ClusterRole`, and `ClusterRoleBinding` that satisfy them are automatically included in the generated manifests.
