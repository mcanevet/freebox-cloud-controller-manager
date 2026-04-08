/*
Copyright 2025.

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

// Package freebox implements the k8s.io/cloud-provider interface for the
// Freebox API.  It provides InstancesV2 support so that the CCM framework can
// set node.spec.providerID on workload cluster nodes.
package freebox

import (
	"fmt"
	"os"
)

// Config holds the credentials and endpoint information needed to talk to the
// Freebox API.
type Config struct {
	// Endpoint is the base URL of the Freebox, e.g. "https://mafreebox.freebox.fr".
	Endpoint string
	// APIVersion is the Freebox API version, e.g. "v8".
	APIVersion string
	// AppID is the registered application identifier, e.g. "fr.freebox.ccm".
	AppID string
	// AppToken is the private application token obtained during initial
	// authorization.
	AppToken string
}

// ConfigFromEnv reads the Freebox CCM configuration from environment variables.
// The following variables are required:
//
//	FREEBOX_ENDPOINT    – base URL of the Freebox
//	FREEBOX_VERSION      – API version (e.g. "v8", "latest")
//	FREEBOX_APP_ID       – application identifier
//	FREEBOX_TOKEN        – private application token
func ConfigFromEnv() (*Config, error) {
	get := func(key string) (string, error) {
		v := os.Getenv(key)
		if v == "" {
			return "", fmt.Errorf("required environment variable %q is not set", key)
		}
		return v, nil
	}

	endpoint, err := get("FREEBOX_ENDPOINT")
	if err != nil {
		return nil, err
	}
	version, err := get("FREEBOX_VERSION")
	if err != nil {
		return nil, err
	}
	appID, err := get("FREEBOX_APP_ID")
	if err != nil {
		return nil, err
	}
	token, err := get("FREEBOX_TOKEN")
	if err != nil {
		return nil, err
	}

	return &Config{
		Endpoint:   endpoint,
		APIVersion: version,
		AppID:      appID,
		AppToken:   token,
	}, nil
}
