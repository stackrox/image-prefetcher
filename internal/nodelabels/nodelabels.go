package nodelabels

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// LabelPrefix is the prefix for all image-prefetcher labels.
	LabelPrefix = "image-prefetcher.stackrox.io/"

	// LabelValueSuccess indicates all images were successfully prefetched.
	LabelValueSuccess = "success"

	// LabelValueFailed indicates one or more images failed to prefetch.
	LabelValueFailed = "failed"

	// MaxLabelNameLength is the maximum length for the label name part (after prefix).
	MaxLabelNameLength = 63
)

// NewClient creates a new Kubernetes client using in-cluster configuration.
func NewClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return clientset, nil
}

// sanitizeLabelName converts an arbitrary string into a valid Kubernetes label name.
// Label names must:
// - Be at most 63 characters.
// - Start and end with alphanumeric characters.
// - Contain only alphanumerics, dashes, underscores, and dots.
func sanitizeLabelName(s string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	sanitized := reg.ReplaceAllString(s, "-")

	if len(sanitized) > MaxLabelNameLength {
		sanitized = sanitized[:MaxLabelNameLength]
	}

	sanitized = strings.Trim(sanitized, "-._")

	if sanitized == "" {
		sanitized = "prefetcher"
	}

	return sanitized
}

// UpdateNodeLabels updates the labels on a node to reflect the overall prefetch status.
// It updates only the label for this specific instance, leaving other instance labels untouched.
func UpdateNodeLabels(ctx context.Context, client kubernetes.Interface, nodeName, instanceName string, results *sync.Map, logger *slog.Logger) error {
	node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

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

	node.Labels[labelKey] = labelValue

	logger.Info("Setting prefetch status label", "key", labelKey, "value", labelValue)

	_, err = client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update node %s: %w", nodeName, err)
	}

	logger.Info("Successfully updated node label", "node", nodeName, "instance", instanceName, "status", labelValue)

	return nil
}
