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
	networking "k8s.io/api/networking/v1"

	"k8s.io/ingress-nginx/internal/ingress/annotations/parser"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
	"k8s.io/klog"
)

const (
	// Ingress version
	IngressVersion = "version"
)

const (
	// Default ingress version
	DefaultIngVer = 0
)

// Config returns checksum configuration for an Ingress rule
type Config struct {
	IngVersion int `json:"ingVersion"`
}

type checksum struct {
	r resolver.Resolver
}

// Equal tests for equality between two Config types
func (checksum1 *Config) Equal(checksum2 *Config) bool {
	if checksum1 == checksum2 {
		return true
	}
	if checksum1.IngVersion != checksum2.IngVersion {
		return false
	}

	return true
}

// NewParser creates a new Checksum annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return checksum{r}
}

// ParseAnnotations parses the annotations contained in the ingress
// rule used to indicate if is required to configure
func (a checksum) Parse(ing *networking.Ingress) (interface{}, error) {
	var err error
	config := &Config{}

	config.IngVersion, err = parser.GetIntAnnotation(IngressVersion, ing)
	if err != nil {
		klog.Infof("Get annotation %s, err: %s", IngressVersion, err)
		config.IngVersion = DefaultIngVer
	}

	return config, nil
}
