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

package secannotations

import (
	"github.com/imdario/mergo"
	"k8s.io/klog"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/ingress-nginx/internal/ingress/errors"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
	"k8s.io/ingress-nginx/internal/ingress/secannotations/checksum"
	"k8s.io/ingress-nginx/internal/ingress/secannotations/parser"
	"k8s.io/ingress-nginx/internal/ingress/secannotations/secretgray"
)

// Secret defines the valid annotations present in one Secret
type Secret struct {
	metav1.ObjectMeta
	SecretGray secretgray.Config
	CheckSum   checksum.Config
}

// Extractor defines the annotation parsers to be used in the extraction of annotations
type Extractor struct {
	annotations map[string]parser.SecretAnnotation
}

// NewAnnotationExtractor creates a new annotations extractor
func NewAnnotationExtractor(cfg resolver.Resolver) Extractor {
	return Extractor{
		map[string]parser.SecretAnnotation{
			"SecretGray": secretgray.NewParser(cfg),
			"CheckSum":   checksum.NewParser(cfg),
		},
	}
}

// Extract extracts the annotations from an Ingress
func (e Extractor) Extract(secret *apiv1.Secret) *Secret {
	pia := &Secret{
		ObjectMeta: secret.ObjectMeta,
	}

	data := make(map[string]interface{})
	for name, annotationParser := range e.annotations {
		val, err := annotationParser.Parse(secret)
		klog.Infof("annotation %v in Secret %v/%v: %v", name, secret.GetNamespace(), secret.GetName(), val)
		if err != nil {
			if errors.IsMissingAnnotations(err) {
				continue
			}

			klog.Infof("error reading %v annotation in Secret %v/%v: %v", name, secret.GetNamespace(), secret.GetName(), err)
		}

		if val != nil {
			data[name] = val
		}
	}

	err := mergo.MapWithOverwrite(pia, data)
	if err != nil {
		klog.Errorf("unexpected error merging extracted annotations of Secret: %v", err)
	}

	return pia
}
