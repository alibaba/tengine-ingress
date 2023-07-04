/*
Copyright 2022 The Alibaba Authors.

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

package checksum

import (
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
	"k8s.io/ingress-nginx/internal/ingress/secannotations/parser"
	"k8s.io/klog"
)

const (
	// Secret version
	SecretVersion = "version"
)

const (
	// Default secret version
	DefaultSecretVer = 0
)

// Config returns checksum configuration for an Secret rule
type Config struct {
	SecretVersion int `json:"secretVersion"`
}

type checksum struct {
	r resolver.Resolver
}

// Equal tests for equality between two Config types
func (checksum1 *Config) Equal(checksum2 *Config) bool {
	if checksum1 == checksum2 {
		return true
	}
	if checksum1.SecretVersion != checksum2.SecretVersion {
		return false
	}

	return true
}

// NewParser creates a new Checksum annotation parser
func NewParser(r resolver.Resolver) parser.SecretAnnotation {
	return checksum{r}
}

// ParseAnnotations parses the annotations contained in the secret
// rule used to indicate if is required to configure
func (a checksum) Parse(secret *apiv1.Secret) (interface{}, error) {
	var err error
	config := &Config{}

	config.SecretVersion, err = parser.GetIntAnnotation(SecretVersion, secret)
	if err != nil {
		klog.Infof("Get annotation %s, err: %s", SecretVersion, err)
		config.SecretVersion = DefaultSecretVer
	}

	return config, nil
}
