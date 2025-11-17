package nodelabels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

const (
	// LabelPrefix is the prefix for all image-prefetcher labels.
	LabelPrefix = "image-prefetcher.stackrox.io/"

	// LabelValueSuccess indicates all images were successfully prefetched.
	LabelValueSuccess = "succeeded"

	// LabelValueFailed indicates one or more images failed to prefetch.
	LabelValueFailed = "failed"
)

// NewClient creates a new Kubernetes node client using in-cluster configuration.
func NewClient() (corev1.NodeInterface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return clientset.CoreV1().Nodes(), nil
}

// sanitizeLabelName converts an arbitrary string into a valid Kubernetes label name.
// Label names must:
// - Be at most 63 characters.
// - Start and end with alphanumeric characters.
// - Contain only alphanumerics, dashes, underscores, and dots.
func sanitizeLabelName(s string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	sanitized := reg.ReplaceAllString(s, "-")

	if len(sanitized) > validation.DNS1123LabelMaxLength {
		sanitized = sanitized[:validation.DNS1123LabelMaxLength]
	}

	sanitized = strings.Trim(sanitized, "._-")

	if sanitized == "" {
		sanitized = "prefetcher"
	}

	return sanitized
}

func retryAll(err error) bool {
	return true
}

// UpdateNodeLabels updates the labels on a node to reflect the overall prefetch status.
// Uses PATCH instead of UPDATE to reduce conflicts and avoid fetching the entire node object.
func UpdateNodeLabels(ctx context.Context, nodeClient corev1.NodeInterface, nodeName, instanceName string, results *sync.Map, logger *slog.Logger) error {
	// Determine overall status: success if ALL images succeeded, failed otherwise.
	labelValue := LabelValueSuccess
	results.Range(func(key, value interface{}) bool {
		if !value.(bool) {
			labelValue = LabelValueFailed
			return false
		}
		return true
	})

	sanitizedInstanceName := sanitizeLabelName(instanceName)
	labelKey := LabelPrefix + sanitizedInstanceName

	logger.Info("Setting prefetch status node label", "key", labelKey, "value", labelValue)

	type patchPayload struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	}

	err := retry.OnError(retry.DefaultBackoff, retryAll, func() error {
		patch := patchPayload{}
		patch.Metadata.Labels = map[string]string{labelKey: labelValue}

		patchBytes, err := json.Marshal(patch)
		if err != nil {
			return fmt.Errorf("failed to marshal patch: %w", err)
		}

		_, err = nodeClient.Patch(ctx, nodeName, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to patch node %s: %w", nodeName, err)
	}

	logger.Info("Successfully updated node label", "node", nodeName, "instance", instanceName, "status", labelValue)
	return nil
}
