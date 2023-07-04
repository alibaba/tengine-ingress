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

package robots

import (
	networking "k8s.io/api/networking/v1beta1"

	"k8s.io/ingress-nginx/internal/ingress/annotations/parser"
	ing_errors "k8s.io/ingress-nginx/internal/ingress/errors"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
)

type robots struct {
	r resolver.Resolver
}

// NewParser creates a new robots annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return robots{r}
}

// ParseAnnotations parses the annotations contained in the ingress
// rule used to indicate if is required to configure
func (a robots) Parse(ing *networking.Ingress) (interface{}, error) {
	if ing.GetAnnotations() == nil {
		return false, ing_errors.ErrMissingAnnotations
	}

	return parser.GetBoolAnnotation("disable-robots", ing)
}
