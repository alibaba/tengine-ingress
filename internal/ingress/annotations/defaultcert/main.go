/*
Copyright 2020 The Alibaba Authors.
Copyright 2018 The Kubernetes Authors.

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

package defaultcert

import (
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/klog"

	"k8s.io/ingress-nginx/internal/ingress/annotations/parser"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
)

type defaultcert struct {
	r resolver.Resolver
}

// NewParser creates a new default cert annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return defaultcert{r}
}

// Config contains the default cert configuration to be used in the Ingress
type Config struct {
	NeedDefault bool
}

// Parse parses the annotations contained in the ingress to use a default cert
func (a defaultcert) Parse(ing *networking.Ingress) (interface{}, error) {
	config := &Config{
		NeedDefault: false,
	}
	var err error

	if ing.GetAnnotations() == nil {
		return config, nil
	}

	config.NeedDefault, err = parser.GetBoolAnnotation("default-cert", ing)
	if err != nil {
		config.NeedDefault = false
	}

	klog.V(3).Infof("default cert config: [%v]", config)

	return config, nil
}
