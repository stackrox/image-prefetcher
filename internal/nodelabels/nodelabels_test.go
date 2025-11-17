package nodelabels

import (
	"context"
	"strings"
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
	tests := []struct {
		name         string
		input        string
		expected     string
		checkExact   bool
		checkPrefix  bool
		prefixExpect string
	}{
		{
			name:       "simple name",
			input:      "my-images",
			expected:   "my-images",
			checkExact: true,
		},
		{
			name:       "with underscores",
			input:      "my_images",
			expected:   "my_images",
			checkExact: true,
		},
		{
			name:       "with dots",
			input:      "my.images",
			expected:   "my.images",
			checkExact: true,
		},
		{
			name:       "with spaces (invalid)",
			input:      "my images",
			expected:   "my-images",
			checkExact: true,
		},
		{
			name:       "starts with dash (invalid)",
			input:      "-my-images",
			expected:   "my-images",
			checkExact: true,
		},
		{
			name:       "ends with dash (invalid)",
			input:      "my-images-",
			expected:   "my-images",
			checkExact: true,
		},
		{
			name:         "too long",
			input:        "this-is-a-very-long-instance-name-that-exceeds-sixty-three-characters-and-should-be-truncated",
			checkPrefix:  true,
			prefixExpect: "this-is-a-very-long-instance-name",
		},
		{
			name:       "special characters",
			input:      "my@images!",
			expected:   "my-images",
			checkExact: true,
		},
		{
			name:       "empty string",
			input:      "",
			expected:   "prefetcher",
			checkExact: true,
		},
		{
			name:       "only invalid chars",
			input:      "!!!",
			expected:   "prefetcher",
			checkExact: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := sanitizeLabelName(tt.input)

			if tt.checkExact {
				assert.Equal(t, tt.expected, output)
			}
			if tt.checkPrefix {
				assert.True(t, strings.HasPrefix(output, tt.prefixExpect))
			}
		})
	}
}

func TestLabelKeyConstruction(t *testing.T) {
	instanceName := "my-images"
	sanitized := sanitizeLabelName(instanceName)
	labelKey := LabelPrefix + sanitized

	assert.Equal(t, "image-prefetcher.stackrox.io/my-images", labelKey)
	assert.True(t, strings.HasPrefix(labelKey, LabelPrefix))
}

func makeSyncMap(m map[string]bool) *sync.Map {
	var sm sync.Map
	for k, v := range m {
		sm.Store(k, v)
	}
	return &sm
}

func TestUpdateNodeLabels(t *testing.T) {
	tests := []struct {
		name           string
		nodeName       string
		instanceName   string
		existingLabels map[string]string
		results        map[string]bool
		expectedLabel  string
		expectError    bool
		nodeExists     bool
	}{
		{
			name:         "all images succeeded",
			nodeName:     "test-node",
			instanceName: "my-images",
			results: map[string]bool{
				"image1": true,
				"image2": true,
				"image3": true,
			},
			expectedLabel: LabelValueSuccess,
			nodeExists:    true,
		},
		{
			name:         "some images failed",
			nodeName:     "test-node",
			instanceName: "my-images",
			results: map[string]bool{
				"image1": true,
				"image2": false,
				"image3": true,
			},
			expectedLabel: LabelValueFailed,
			nodeExists:    true,
		},
		{
			name:         "all images failed",
			nodeName:     "test-node",
			instanceName: "my-images",
			results: map[string]bool{
				"image1": false,
				"image2": false,
			},
			expectedLabel: LabelValueFailed,
			nodeExists:    true,
		},
		{
			name:          "empty results shows success",
			nodeName:      "test-node",
			instanceName:  "my-images",
			results:       map[string]bool{},
			expectedLabel: LabelValueSuccess,
			nodeExists:    true,
		},
		{
			name:         "updates existing label",
			nodeName:     "test-node",
			instanceName: "my-images",
			existingLabels: map[string]string{
				"image-prefetcher.stackrox.io/my-images": LabelValueFailed,
			},
			results: map[string]bool{
				"image1": true,
			},
			expectedLabel: LabelValueSuccess,
			nodeExists:    true,
		},
		{
			name:         "preserves other labels",
			nodeName:     "test-node",
			instanceName: "my-images",
			existingLabels: map[string]string{
				"kubernetes.io/hostname": "test-node",
				"other-label":            "value",
			},
			results: map[string]bool{
				"image1": true,
			},
			expectedLabel: LabelValueSuccess,
			nodeExists:    true,
		},
		{
			name:         "creates labels on node with no labels",
			nodeName:     "test-node",
			instanceName: "my-images",
			results: map[string]bool{
				"image1": true,
			},
			expectedLabel: LabelValueSuccess,
			nodeExists:    true,
		},
		{
			name:         "sanitizes instance name",
			nodeName:     "test-node",
			instanceName: "my images!",
			results: map[string]bool{
				"image1": true,
			},
			expectedLabel: LabelValueSuccess,
			nodeExists:    true,
		},
		{
			name:         "node not found returns error",
			nodeName:     "nonexistent-node",
			instanceName: "my-images",
			results: map[string]bool{
				"image1": true,
			},
			expectError: true,
			nodeExists:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fakeClient *fake.Clientset
			if tt.nodeExists {
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   tt.nodeName,
						Labels: tt.existingLabels,
					},
				}
				fakeClient = fake.NewSimpleClientset(node)
			} else {
				fakeClient = fake.NewSimpleClientset()
			}

			results := makeSyncMap(tt.results)
			logger := slogt.New(t)
			ctx := context.Background()

			err := UpdateNodeLabels(ctx, fakeClient, tt.nodeName, tt.instanceName, results, logger)

			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			node, err := fakeClient.CoreV1().Nodes().Get(ctx, tt.nodeName, metav1.GetOptions{})
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
