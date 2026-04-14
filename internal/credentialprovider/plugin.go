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

	credentialproviderv1 "k8s.io/kubelet/pkg/apis/credentialprovider/v1"
)

const supportedAPIVersion = "kubelet.k8s.io/v1"

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

// credentialProviderConfig represents the credential provider configuration file.
type credentialProviderConfig struct {
	APIVersion string                    `json:"apiVersion"`
	Kind       string                    `json:"kind"`
	Providers  []credentialProviderEntry `json:"providers"`
}

type credentialProviderEntry struct {
	Name                 string   `json:"name"`
	APIVersion           string   `json:"apiVersion"`
	MatchImages          []string `json:"matchImages"`
	Args                 []string `json:"args,omitempty"`
	DefaultCacheDuration string   `json:"defaultCacheDuration,omitempty"`
	Env                  []envVar `json:"env,omitempty"`
	TokenAttributes      []string `json:"tokenAttributes,omitempty"`
}

type envVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
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

// readCredentialProviderConfig reads and parses the credential provider config file.
func readCredentialProviderConfig(configPath string) (*credentialProviderConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config credentialProviderConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if config.Kind != "CredentialProviderConfig" {
		return nil, fmt.Errorf("unexpected kind %q, expected CredentialProviderConfig", config.Kind)
	}

	return &config, nil
}

// Lookup is like Lookup(context.Background(), ...).
func (kr *PluginKeyring) Lookup(image string) ([]AuthConfig, bool) {
	return kr.LookupWithCtx(context.Background(), image)
}

// LookupWithCtx returns credentials for the given image from matching plugins.
func (kr *PluginKeyring) LookupWithCtx(ctx context.Context, image string) ([]AuthConfig, bool) {
	if kr == nil {
		return nil, false
	}

	var allCreds []AuthConfig
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

		allCreds = append(allCreds, creds...)
	}

	if len(allCreds) > 0 {
		return allCreds, true
	}

	return nil, false
}

// LookupForKeyring returns credentials formatted for the DockerKeyring interface.
func (kr *PluginKeyring) LookupForKeyring(image string) DockerConfig {
	creds, ok := kr.Lookup(image)
	if !ok {
		return DockerConfig{}
	}

	cfg := DockerConfig{}
	for _, cred := range creds {
		registry := cred.ServerAddress
		if registry == "" {
			registry = image
		}
		cfg[registry] = DockerConfigEntry{
			Username: cred.Username,
			Password: cred.Password,
		}
	}
	return cfg
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

// execPlugin executes the credential provider plugin and parses the response.
func (kr *PluginKeyring) execPlugin(ctx context.Context, provider pluginProviderWrapper, image string) ([]AuthConfig, error) {
	// Prepare the request
	request := credentialproviderv1.CredentialProviderRequest{
		Image: image,
	}
	requestJSON, err := json.Marshal(request)
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

	if response.APIVersion != supportedAPIVersion {
		return nil, fmt.Errorf("apiVersion from credential plugin response did not match expected apiVersion:%s, actual apiVersion:%s", supportedAPIVersion, response.APIVersion)
	}

	// Convert to AuthConfig
	var creds []AuthConfig
	for registry, authConfig := range response.Auth {
		creds = append(creds, AuthConfig{
			Username:      authConfig.Username,
			Password:      authConfig.Password,
			ServerAddress: registry,
		})
	}

	kr.logger.Debug("received credentials from plugin", "plugin", provider.name, "count", len(creds))
	return creds, nil
}
