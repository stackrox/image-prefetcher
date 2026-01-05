package nodelabels

import (
	"context"
	"sync"
	"testing"

	"github.com/neilotoole/slogt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSanitizeLabelName(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"simple name": {
			input:    "my-images",
			expected: "my-images",
		},
		"with underscores": {
			input:    "my_images",
			expected: "my_images",
		},
		"with dots": {
			input:    "my.images",
			expected: "my.images",
		},
		"with spaces (invalid)": {
			input:    "my images",
			expected: "my-images",
		},
		"starts with dash (invalid)": {
			input:    "-my-images",
			expected: "my-images",
		},
		"ends with dash (invalid)": {
			input:    "my-images-",
			expected: "my-images",
		},
		"too long": {
			input:    "this-is-a-very-long-instance-name-that-exceeds-sixty-three-characters-and-should-be-truncated",
			expected: "this-is-a-very-long-instance-name-that-exceeds-sixty-three-char",
		},
		"special characters": {
			input:    "my@images!",
			expected: "my-images",
		},
		"empty string": {
			input:    "",
			expected: "prefetcher",
		},
		"only invalid chars": {
			input:    "!!!",
			expected: "prefetcher",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			output := sanitizeLabelName(tt.input)
			assert.Equal(t, tt.expected, output)
		})
	}
}

func makeSyncMap(m map[string]bool) *sync.Map {
	var sm sync.Map
	for k, v := range m {
		sm.Store(k, v)
	}
	return &sm
}

func TestUpdateNodeLabels(t *testing.T) {
	tests := map[string]struct {
		instanceName   string
		existingLabels map[string]string
		results        map[string]bool
		expectedLabel  string
		nodeMissing    bool
	}{
		"all images succeeded": {
			instanceName: "my-images",
			results: map[string]bool{
				"image1": true,
				"image2": true,
				"image3": true,
			},
			expectedLabel: LabelValueSuccess,
		},
		"some images failed": {
			instanceName: "my-images",
			results: map[string]bool{
				"image1": true,
				"image2": false,
				"image3": true,
			},
			expectedLabel: LabelValueFailed,
		},
		"empty results shows success": {
			instanceName:  "my-images",
			results:       map[string]bool{},
			expectedLabel: LabelValueSuccess,
		},
		"updates existing label": {
			instanceName: "my-images",
			existingLabels: map[string]string{
				"image-prefetcher.stackrox.io/my-images": LabelValueFailed,
			},
			results: map[string]bool{
				"image1": true,
			},
			expectedLabel: LabelValueSuccess,
		},
		"preserves other labels": {
			instanceName: "my-images",
			existingLabels: map[string]string{
				"kubernetes.io/hostname": "my-images",
				"other-label":            "value",
			},
			results: map[string]bool{
				"image1": true,
			},
			expectedLabel: LabelValueSuccess,
		},
		"sanitizes instance name": {
			instanceName: "my images!",
			results: map[string]bool{
				"image1": true,
			},
			expectedLabel: LabelValueSuccess,
		},
		"node not found returns error": {
			instanceName: "my-images",
			results: map[string]bool{
				"image1": true,
			},
			nodeMissing: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var fakeClient *fake.Clientset
			if !tt.nodeMissing {
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   name,
						Labels: tt.existingLabels,
					},
				}
				fakeClient = fake.NewClientset(node)
			} else {
				fakeClient = fake.NewClientset()
			}

			results := makeSyncMap(tt.results)
			logger := slogt.New(t)
			ctx := context.Background()

			nodeClient := fakeClient.CoreV1().Nodes()
			labels := generatePrefetchStatusLabels(tt.instanceName, results)
			err := patchNodeLabelsWithClient(ctx, nodeClient, name, labels, logger)

			if tt.nodeMissing {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			node, err := nodeClient.Get(ctx, name, metav1.GetOptions{})
			require.NoError(t, err)

			sanitizedInstanceName := sanitizeLabelName(tt.instanceName)
			expectedLabelKey := LabelPrefix + sanitizedInstanceName

			assert.Equal(t, tt.expectedLabel, node.Labels[expectedLabelKey])

			for k, v := range tt.existingLabels {
				if k != expectedLabelKey {
					assert.Equal(t, v, node.Labels[k], "existing label should be preserved")
				}
			}
		})
	}
}
