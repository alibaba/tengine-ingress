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

package secretgray

import (
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
	"k8s.io/ingress-nginx/internal/ingress/secannotations/parser"
	"k8s.io/klog"
)

const (
	// Secret flag
	SecretGrayFlag = "secret-rollout"
	// Secret current version
	SecretGrayCurVer = "secret-rollout-current-revision"
	// Secret new version
	SecretGrayNewVer = "secret-rollout-update-revision"
	// For a StatefulSet with N replicas,
	// each Pod in the StatefulSet will be assigned an integer ordinal,
	// from 0 up through N-1, that is unique over the Set.
	// If pod ordinal is less than index, the pod will process the new secret.
	SecretGrayIndex = "secret-rollout-index-id"
)

const (
	// Gray process is not start
	PodIndexEmpty = 0
	// Gary process is done
	PodIndexDone = -1
)

// Config returns gray configuration for an Secret
type Config struct {
	SecGrayFlag   bool   `json:"secretGrayFlag"`
	SecGrayCurVer string `json:"secretGrayCurVer"`
	SecGrayNewVer string `json:"secretGrayNewVer"`
	SecGrayIndex  int    `json:"secretGrayIndex"`
}

type secretgray struct {
	r resolver.Resolver
}

// Equal tests for equality between two Config types
func (gray1 *Config) Equal(gray2 *Config) bool {
	if gray1 == gray2 {
		return true
	}
	if gray1 == nil || gray2 == nil {
		return false
	}
	if gray1.SecGrayFlag != gray2.SecGrayFlag {
		return false
	}
	if gray1.SecGrayCurVer != gray2.SecGrayCurVer {
		return false
	}
	if gray1.SecGrayNewVer != gray2.SecGrayNewVer {
		return false
	}
	if gray1.SecGrayIndex != gray2.SecGrayIndex {
		return false
	}

	return true
}

// NewParser creates a new Gray annotation parser
func NewParser(r resolver.Resolver) parser.SecretAnnotation {
	return secretgray{r}
}

// ParseAnnotations parses the annotations contained in the
// secret used to indicate if is required to configure
func (a secretgray) Parse(secret *apiv1.Secret) (interface{}, error) {
	var err error
	config := &Config{}

	config.SecGrayFlag, err = parser.GetBoolAnnotation(SecretGrayFlag, secret)
	if err != nil {
		klog.Infof("Get annotation %s, err: %s", SecretGrayFlag, err)
		config.SecGrayFlag = false
	}

	config.SecGrayCurVer, err = parser.GetStringAnnotation(SecretGrayCurVer, secret)
	if err != nil {
		klog.Infof("Get annotation %s, err: %s", SecretGrayCurVer, err)
		config.SecGrayCurVer = ""
	}

	config.SecGrayNewVer, err = parser.GetStringAnnotation(SecretGrayNewVer, secret)
	if err != nil {
		klog.Infof("Get annotation %s, err: %s", SecretGrayNewVer, err)
		config.SecGrayNewVer = ""
	}

	config.SecGrayIndex, err = parser.GetIntAnnotation(SecretGrayIndex, secret)
	if err != nil {
		klog.Infof("Get annotation %s, err: %s", SecretGrayIndex, err)
		config.SecGrayIndex = PodIndexEmpty
	}

	return config, nil
}
