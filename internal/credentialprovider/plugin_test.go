package credentialprovider

import (
	"github.com/neilotoole/slogt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path"
	"testing"
)

func TestPluginKeyring_execPlugin(t *testing.T) {
	tests := map[string]struct {
		requestFile  string
		responseFile string
		exitCode     string

		want    DockerConfig
		wantErr bool
	}{
		"empty-success": {
			requestFile:  "basic-request.json",
			responseFile: "basic-response.json",
			exitCode:     "0",

			want: DockerConfig{
				"foo": {
					Username: "user",
					Password: "not-a-secret",
					Email:    "",
					Provider: nil,
				},
			},
			wantErr: false,
		},
		"empty-error": {
			requestFile:  "basic-request.json",
			responseFile: "basic-response.json",
			exitCode:     "1",

			wantErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			actualRequestFile := path.Join(t.TempDir(), tt.requestFile)
			provider := pluginProviderWrapper{
				name:        "fake_plugin",
				binPath:     "test_data/fake_plugin",
				matchImages: []string{"*"},
				// fake_plugin's args are: path to save stdin to, path to copy to stdout, exit code
				args: []string{actualRequestFile, path.Join("test_data", tt.responseFile), tt.exitCode},
			}
			kr := &PluginKeyring{logger: slogt.New(t)}
			got, err := kr.execPlugin(t.Context(), provider, "foo-image")
			if (err != nil) != tt.wantErr {
				t.Errorf("execPlugin() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			actualRequest, err := os.ReadFile(actualRequestFile)
			require.NoError(t, err)
			expectedRequest, err := os.ReadFile(path.Join("test_data", tt.requestFile))
			require.NoError(t, err)
			assert.Equal(t, string(expectedRequest), string(actualRequest))
			assert.Equal(t, tt.want, got)
		})
	}
}
