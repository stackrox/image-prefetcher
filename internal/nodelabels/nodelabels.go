package nodelabels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
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

// PatchNodeLabels creates a Kubernetes client and updates node labels with prefetch results.
// This function combines client initialization and label updates in a single operation.
// If environment variables are not set or client creation fails, it logs a warning and returns without error.
func PatchNodeLabels(ctx context.Context, results *sync.Map, logger *slog.Logger) error {
	nodeName := os.Getenv("NODE_NAME")
	instanceName := os.Getenv("INSTANCE_NAME")

	if nodeName == "" {
		logger.Info("NODE_NAME environment variable not set, skipping node labeling")
		return nil
	}
	if instanceName == "" {
		logger.Info("INSTANCE_NAME environment variable not set, skipping node labeling")
		return nil
	}

	nodeClient, err := NewClient()
	if err != nil {
		logger.Warn("failed to create Kubernetes client, skipping node labeling", "error", err)
		return nil
	}

	logger.Info("Kubernetes client initialized for node labeling", "node", nodeName, "instance", instanceName)

	// Generate labels based on prefetch results
	labels := generatePrefetchStatusLabels(instanceName, results)

	if err := patchNodeLabelsWithClient(ctx, nodeClient, nodeName, labels, logger); err != nil {
		return fmt.Errorf("failed to update node labels: %w", err)
	}

	return nil
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

// generatePrefetchStatusLabels creates a map of labels based on prefetch results.
// This is a pure function that determines the label key and value without side effects.
func generatePrefetchStatusLabels(instanceName string, results *sync.Map) map[string]string {
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

	return map[string]string{labelKey: labelValue}
}

func retryAll(err error) bool {
	return true
}

// patchNodeLabelsWithClient updates the labels on a node with the provided label map.
// Uses PATCH instead of UPDATE to reduce conflicts and avoid fetching the entire node object.
func patchNodeLabelsWithClient(ctx context.Context, nodeClient corev1.NodeInterface, nodeName string, labels map[string]string, logger *slog.Logger) error {
	logger.Debug("Setting prefetch status node labels", "labels", labels)

	type patchPayload struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	}
	patch := patchPayload{}
	patch.Metadata.Labels = labels
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	err = retry.OnError(retry.DefaultBackoff, retryAll, func() error {
		_, err := nodeClient.Patch(ctx, nodeName, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to patch node %s: %w", nodeName, err)
	}

	logger.Info("Successfully updated node labels", "node", nodeName, "labels", labels)
	return nil
}
