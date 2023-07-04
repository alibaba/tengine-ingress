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

package referrer

import (
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/klog"

	"k8s.io/ingress-nginx/internal/ingress/annotations/parser"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
)

const (
	// Ingress referrer
	IngressReferrer = "ingress-referrer"
)

type referrer struct {
	r resolver.Resolver
}

// Config returns referrer for an Ingress rule
type Config struct {
	IngReferrer string `json:"ingReferrer"`
}

// NewParser creates a new referrer annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return referrer{r}
}

// ParseAnnotations parses the annotations contained in the ingress
// rule used to indicate if is required to configure
func (a referrer) Parse(ing *networking.Ingress) (interface{}, error) {
	var err error
	config := &Config{}

	config.IngReferrer, err = parser.GetStringAnnotation(IngressReferrer, ing)
	if err != nil {
		klog.Infof("Get annotation %s, err: %s", IngressReferrer, err)
		config.IngReferrer = ""
	}

	return config, nil
}
