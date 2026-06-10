package credentialprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/kubelet/pkg/apis/credentialprovider/install"
	credentialproviderv1 "k8s.io/kubelet/pkg/apis/credentialprovider/v1"
	"sigs.k8s.io/yaml"
)

const (
	supportedResponseAPIVersion = "credentialprovider.kubelet.k8s.io/v1"
	supportedConfigAPIVersion   = "kubelet.config.k8s.io/v1"
)

// Minimal mirrors of k8s.io/kubelet/config/v1.CredentialProviderConfig to avoid depending on k8s.io/kubernetes.
type credentialProviderConfig struct {
	APIVersion string               `json:"apiVersion"`
	Kind       string               `json:"kind"`
	Providers  []credentialProvider `json:"providers"`
}

type credentialProvider struct {
	Name        string   `json:"name"`
	MatchImages []string `json:"matchImages"`
	APIVersion  string   `json:"apiVersion"`
	Args        []string `json:"args,omitempty"`
}

// PluginKeyring wraps the credential provider plugin functionality.
type PluginKeyring struct {
	providers []pluginProviderWrapper
	logger    *slog.Logger
}

type pluginProviderWrapper struct {
	name        string
	binPath     string
	apiVersion  string
	matchImages []string
	args        []string
}

var (
	scheme = runtime.NewScheme()
	codecs = serializer.NewCodecFactory(scheme)
)

func init() {
	install.Install(scheme)
}

// NewPluginKeyring creates a new keyring that uses credential provider plugins.
func NewPluginKeyring(logger *slog.Logger, configPath, binDir string) (*PluginKeyring, error) {
	if configPath == "" || binDir == "" {
		return nil, nil
	}

	config, err := readCredentialProviderConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read credential provider config: %w", err)
	}

	kr := &PluginKeyring{
		providers: make([]pluginProviderWrapper, 0, len(config.Providers)),
		logger:    logger,
	}

	for _, provider := range config.Providers {
		// Find the plugin binary
		pluginBin, err := exec.LookPath(filepath.Join(binDir, provider.Name))
		if err != nil {
			return nil, fmt.Errorf("plugin binary %s not found in %s: %w", provider.Name, binDir, err)
		}

		kr.providers = append(kr.providers, pluginProviderWrapper{
			name:        provider.Name,
			binPath:     pluginBin,
			apiVersion:  provider.APIVersion,
			matchImages: provider.MatchImages,
			args:        provider.Args,
		})
	}

	logger.Info("initialized credential provider plugins", "count", len(kr.providers))
	return kr, nil
}

// readCredentialProviderConfig reads and decodes the credential provider config file.
// Supports both YAML and JSON formats.
func readCredentialProviderConfig(configPath string) (*credentialProviderConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config credentialProviderConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	if config.Kind != "CredentialProviderConfig" {
		return nil, fmt.Errorf("unexpected kind %q, expected CredentialProviderConfig", config.Kind)
	}

	if config.APIVersion != supportedConfigAPIVersion {
		return nil, fmt.Errorf("unexpected API version %q, expected %q", config.APIVersion, supportedConfigAPIVersion)
	}

	return &config, nil
}

// Lookup is like LookupWithCtx(context.Background(), ...).
func (kr *PluginKeyring) Lookup(image string) ([]AuthConfig, bool) {
	return kr.LookupWithCtx(context.Background(), image)
}

// LookupWithCtx returns credentials for the given image from matching plugins.
func (kr *PluginKeyring) LookupWithCtx(ctx context.Context, image string) ([]AuthConfig, bool) {
	if kr == nil {
		return nil, false
	}

	dk := &BasicDockerKeyring{}
	for _, provider := range kr.providers {
		if !kr.matchesImage(provider.matchImages, image) {
			continue
		}

		kr.logger.Debug("executing credential provider plugin", "plugin", provider.name, "image", image)
		creds, err := kr.execPlugin(ctx, provider, image)
		if err != nil {
			kr.logger.Warn("credential provider plugin failed", "plugin", provider.name, "image", image, "error", err)
			continue
		}
		dk.Add(creds)
	}

	return dk.Lookup(image)
}

// matchesImage checks if any of the match patterns match the given image.
func (kr *PluginKeyring) matchesImage(patterns []string, image string) bool {
	for _, pattern := range patterns {
		// Use the same matching logic as kubernetes
		if matched, _ := URLsMatchStr(pattern, image); matched {
			return true
		}
	}
	return false
}

// marshalRequest serializes the request in a format which contains the required apiVersion and kind fields.
func marshalRequest(request *credentialproviderv1.CredentialProviderRequest) ([]byte, error) {
	jsonMediaType := "application/json"
	info, ok := runtime.SerializerInfoForMediaType(codecs.SupportedMediaTypes(), jsonMediaType)
	if !ok {
		return nil, fmt.Errorf("unsupported media type %q", jsonMediaType)
	}
	return runtime.Encode(codecs.EncoderForVersion(info.Serializer, credentialproviderv1.SchemeGroupVersion), request)
}

// execPlugin executes the credential provider plugin and parses the responseFile.
func (kr *PluginKeyring) execPlugin(ctx context.Context, provider pluginProviderWrapper, image string) (DockerConfig, error) {
	// Prepare the request
	request := credentialproviderv1.CredentialProviderRequest{
		Image: image,
	}
	requestJSON, err := marshalRequest(&request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Execute the plugin, use the same timeout as kubelet does.
	timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, provider.binPath, provider.args...)
	cmd.Stdin = bytes.NewReader(requestJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("plugin execution failed: %w, stderr: %s", err, stderr.String())
	}

	// Parse the response
	var response credentialproviderv1.CredentialProviderResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("failed to parse plugin response: %w", err)
	}

	if response.APIVersion != supportedResponseAPIVersion {
		return nil, fmt.Errorf("apiVersion from credential plugin response did not match expected apiVersion:%s, actual apiVersion:%s", supportedResponseAPIVersion, response.APIVersion)
	}

	kr.logger.Debug("received credentials from plugin", "plugin", provider.name, "count", len(response.Auth))
	dockerConfig := make(DockerConfig, len(response.Auth))
	for matchImage, authConfig := range response.Auth {
		dockerConfig[matchImage] = DockerConfigEntry{
			Username: authConfig.Username,
			Password: authConfig.Password,
		}
	}
	return dockerConfig, nil
}
