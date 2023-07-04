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

package gray

import (
	networking "k8s.io/api/networking/v1beta1"

	"k8s.io/ingress-nginx/internal/ingress/annotations/parser"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
	"k8s.io/klog"
)

const (
	// Ingress flag
	IngressGrayFlag = "ingress-rollout"
	// Ingress current version
	IngressGrayCurVer = "ingress-rollout-current-revision"
	// Ingress new version
	IngressGrayNewVer = "ingress-rollout-update-revision"
	// For a StatefulSet with N replicas,
	// each Pod in the StatefulSet will be assigned an integer ordinal,
	// from 0 up through N-1, that is unique over the Set.
	// If pod ordinal is less than index, the pod will process the new ingress.
	IngressGrayIndex = "ingress-rollout-index-id"
)

const (
	// Index empty
	PodIndexEmpty = -1
)

// Config returns gray configuration for an Ingress rule
type Config struct {
	IngGrayFlag   bool   `json:"ingGrayFlag"`
	IngGrayCurVer string `json:"ingGrayCurVer"`
	IngGrayNewVer string `json:"ingGrayNewVer"`
	IngGrayIndex  int    `json:"ingGrayIndex"`
}

type gray struct {
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
	if gray1.IngGrayFlag != gray2.IngGrayFlag {
		return false
	}
	if gray1.IngGrayCurVer != gray2.IngGrayCurVer {
		return false
	}
	if gray1.IngGrayNewVer != gray2.IngGrayNewVer {
		return false
	}
	if gray1.IngGrayIndex != gray2.IngGrayIndex {
		return false
	}

	return true
}

// NewParser creates a new Gray annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return gray{r}
}

// ParseAnnotations parses the annotations contained in the ingress
// rule used to indicate if is required to configure
func (a gray) Parse(ing *networking.Ingress) (interface{}, error) {
	var err error
	config := &Config{}
	parser.AnnotationsPrefix = "tengine.taobao.org"

	config.IngGrayFlag, err = parser.GetBoolAnnotation(IngressGrayFlag, ing)
	if err != nil {
		klog.Infof("Get annotation %s, err: %s", IngressGrayFlag, err)
		config.IngGrayFlag = false
	}

	config.IngGrayCurVer, err = parser.GetStringAnnotation(IngressGrayCurVer, ing)
	if err != nil {
		klog.Infof("Get annotation %s, err: %s", IngressGrayCurVer, err)
		config.IngGrayCurVer = ""
	}

	config.IngGrayNewVer, err = parser.GetStringAnnotation(IngressGrayNewVer, ing)
	if err != nil {
		klog.Infof("Get annotation %s, err: %s", IngressGrayNewVer, err)
		config.IngGrayNewVer = ""
	}

	config.IngGrayIndex, err = parser.GetIntAnnotation(IngressGrayIndex, ing)
	if err != nil {
		klog.Infof("Get annotation %s, err: %s", IngressGrayIndex, err)
		config.IngGrayIndex = PodIndexEmpty
	}

	parser.AnnotationsPrefix = "nginx.ingress.kubernetes.io"

	return config, nil
}
