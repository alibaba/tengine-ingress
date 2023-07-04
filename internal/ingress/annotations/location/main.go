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

package location

import (
	"regexp"
	"strings"

	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/klog"

	"k8s.io/ingress-nginx/internal/ingress/annotations/parser"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
)

var (
	validPreceding = regexp.MustCompile(`^(=|~|~\*|\^~)$`)
)

type location struct {
	r resolver.Resolver
}

// Config returns the configuration rules for setting up the Location
type Config struct {
	LocationPreceding  string
	LocationPathPrefix string
	LocationPathEscape bool
}

// NewParser creates a new location preceding annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return location{r}
}

// ParseAnnotations parses the annotations contained in the ingress
// rule used to indicate the location preceding.
func (a location) Parse(ing *networking.Ingress) (interface{}, error) {
	config := &Config{
		LocationPreceding:  "",
		LocationPathPrefix: "",
		LocationPathEscape: false,
	}

	if ing.GetAnnotations() == nil {
		return config, nil
	}

	var err error
	var preceding string

	preceding, err = parser.GetStringAnnotation("location-preceding", ing)
	if err != nil {
		preceding = ""
	}

	preceding = strings.TrimSpace(preceding)
	if preceding != "" && !validPreceding.MatchString(preceding) {
		klog.Warningf("Location preceding %v is not a valid value for the location preceding annotation. just using default \"\"", preceding)
		preceding = ""
	}

	config.LocationPreceding = preceding

	var pathPrefix string

	pathPrefix, err = parser.GetStringAnnotation("location-path-prefix", ing)
	if err != nil {
		pathPrefix = ""
	}

	pathPrefix = strings.TrimSpace(pathPrefix)
	config.LocationPathPrefix = pathPrefix

	config.LocationPathEscape, err = parser.GetBoolAnnotation("location-path-escape", ing)
	if err != nil {
		config.LocationPathEscape = false
	}

	klog.V(3).Infof("location config: [%v]", config)
	return config, nil
}
