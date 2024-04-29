/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package credentialprovider

import (
	"reflect"
	"testing"
)

func TestURLsMatch(t *testing.T) {
	tests := []struct {
		globURL       string
		targetURL     string
		matchExpected bool
	}{
		// match when there is no path component
		{
			globURL:       "*.kubernetes.io",
			targetURL:     "prefix.kubernetes.io",
			matchExpected: true,
		},
		{
			globURL:       "prefix.*.io",
			targetURL:     "prefix.kubernetes.io",
			matchExpected: true,
		},
		{
			globURL:       "prefix.kubernetes.*",
			targetURL:     "prefix.kubernetes.io",
			matchExpected: true,
		},
		{
			globURL:       "*-good.kubernetes.io",
			targetURL:     "prefix-good.kubernetes.io",
			matchExpected: true,
		},
		// match with path components
		{
			globURL:       "*.kubernetes.io/blah",
			targetURL:     "prefix.kubernetes.io/blah",
			matchExpected: true,
		},
		{
			globURL:       "prefix.*.io/foo",
			targetURL:     "prefix.kubernetes.io/foo/bar",
			matchExpected: true,
		},
		// match with path components and ports
		{
			globURL:       "*.kubernetes.io:1111/blah",
			targetURL:     "prefix.kubernetes.io:1111/blah",
			matchExpected: true,
		},
		{
			globURL:       "prefix.*.io:1111/foo",
			targetURL:     "prefix.kubernetes.io:1111/foo/bar",
			matchExpected: true,
		},
		// no match when number of parts mismatch
		{
			globURL:       "*.kubernetes.io",
			targetURL:     "kubernetes.io",
			matchExpected: false,
		},
		{
			globURL:       "*.*.kubernetes.io",
			targetURL:     "prefix.kubernetes.io",
			matchExpected: false,
		},
		{
			globURL:       "*.*.kubernetes.io",
			targetURL:     "kubernetes.io",
			matchExpected: false,
		},
		{
			globURL:       "*kubernetes.io",
			targetURL:     "a.kubernetes.io",
			matchExpected: false,
		},
		// match when number of parts match
		{
			globURL:       "*kubernetes.io",
			targetURL:     "kubernetes.io",
			matchExpected: true,
		},
		{
			globURL:       "*.*.*.kubernetes.io",
			targetURL:     "a.b.c.kubernetes.io",
			matchExpected: true,
		},
		// no match when some parts mismatch
		{
			globURL:       "kubernetes.io",
			targetURL:     "kubernetes.com",
			matchExpected: false,
		},
		{
			globURL:       "k*.io",
			targetURL:     "quay.io",
			matchExpected: false,
		},
		// no match when ports mismatch
		{
			globURL:       "*.kubernetes.io:1234/blah",
			targetURL:     "prefix.kubernetes.io:1111/blah",
			matchExpected: false,
		},
		{
			globURL:       "prefix.*.io/foo",
			targetURL:     "prefix.kubernetes.io:1111/foo/bar",
			matchExpected: false,
		},
	}
	for _, test := range tests {
		matched, _ := URLsMatchStr(test.globURL, test.targetURL)
		if matched != test.matchExpected {
			t.Errorf("Expected match result of %s and %s to be %t, but was %t",
				test.globURL, test.targetURL, test.matchExpected, matched)
		}
	}
}

func TestIsDefaultRegistryMatch(t *testing.T) {
	samples := []map[bool]string{
		{true: "foo/bar"},
		{true: "docker.io/foo/bar"},
		{true: "index.docker.io/foo/bar"},
		{true: "foo"},
		{false: ""},
		{false: "registry.tld/foo/bar"},
		{false: "registry:5000/foo/bar"},
		{false: "myhostdocker.io/foo/bar"},
	}
	for _, sample := range samples {
		for expected, imageName := range sample {
			if got := isDefaultRegistryMatch(imageName); got != expected {
				t.Errorf("Expected '%s' to be %t, got %t", imageName, expected, got)
			}
		}
	}
}

func TestDockerKeyringLookup(t *testing.T) {
	ada := AuthConfig{
		Username: "ada",
		Password: "smash", // Fake value for testing.
		Email:    "ada@example.com",
	}

	grace := AuthConfig{
		Username: "grace",
		Password: "squash", // Fake value for testing.
		Email:    "grace@example.com",
	}

	dk := &BasicDockerKeyring{}
	dk.Add(DockerConfig{
		"bar.example.com/pong": DockerConfigEntry{
			Username: grace.Username,
			Password: grace.Password,
			Email:    grace.Email,
		},
		"bar.example.com": DockerConfigEntry{
			Username: ada.Username,
			Password: ada.Password,
			Email:    ada.Email,
		},
	})

	tests := []struct {
		image string
		match []AuthConfig
		ok    bool
	}{
		// direct match
		{"bar.example.com", []AuthConfig{ada}, true},

		// direct match deeper than other possible matches
		{"bar.example.com/pong", []AuthConfig{grace, ada}, true},

		// no direct match, deeper path ignored
		{"bar.example.com/ping", []AuthConfig{ada}, true},

		// match first part of path token
		{"bar.example.com/pongz", []AuthConfig{grace, ada}, true},

		// match regardless of sub-path
		{"bar.example.com/pong/pang", []AuthConfig{grace, ada}, true},

		// no host match
		{"example.com", []AuthConfig{}, false},
		{"foo.example.com", []AuthConfig{}, false},
	}

	for i, tt := range tests {
		match, ok := dk.Lookup(tt.image)
		if tt.ok != ok {
			t.Errorf("case %d: expected ok=%t, got %t", i, tt.ok, ok)
		}

		if !reflect.DeepEqual(tt.match, match) {
			t.Errorf("case %d: expected match=%#v, got %#v", i, tt.match, match)
		}
	}
}

// This validates that dockercfg entries with a scheme and url path are properly matched
// by images that only match the hostname.
// NOTE: the above covers the case of a more specific match trumping just hostname.
func TestIssue3797(t *testing.T) {
	rex := AuthConfig{
		Username: "rex",
		Password: "tiny arms", // Fake value for testing.
		Email:    "rex@example.com",
	}

	dk := &BasicDockerKeyring{}
	dk.Add(DockerConfig{
		"https://quay.io/v1/": DockerConfigEntry{
			Username: rex.Username,
			Password: rex.Password,
			Email:    rex.Email,
		},
	})

	tests := []struct {
		image string
		match []AuthConfig
		ok    bool
	}{
		// direct match
		{"quay.io", []AuthConfig{rex}, true},

		// partial matches
		{"quay.io/foo", []AuthConfig{rex}, true},
		{"quay.io/foo/bar", []AuthConfig{rex}, true},
	}

	for i, tt := range tests {
		match, ok := dk.Lookup(tt.image)
		if tt.ok != ok {
			t.Errorf("case %d: expected ok=%t, got %t", i, tt.ok, ok)
		}

		if !reflect.DeepEqual(tt.match, match) {
			t.Errorf("case %d: expected match=%#v, got %#v", i, tt.match, match)
		}
	}
}
