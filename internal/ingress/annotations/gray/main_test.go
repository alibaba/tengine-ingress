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
	"strconv"
	"testing"

	api "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/ingress-nginx/internal/ingress/annotations/parser"
	"k8s.io/ingress-nginx/internal/ingress/resolver"

	"k8s.io/apimachinery/pkg/util/intstr"
)

func buildIngress() *networking.Ingress {
	defaultBackend := networking.IngressBackend{
		ServiceName: "default-backend",
		ServicePort: intstr.FromInt(80),
	}

	return &networking.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "foo",
			Namespace: api.NamespaceDefault,
		},
		Spec: networking.IngressSpec{
			Backend: &networking.IngressBackend{
				ServiceName: "default-backend",
				ServicePort: intstr.FromInt(80),
			},
			Rules: []networking.IngressRule{
				{
					Host: "foo.bar.com",
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path:    "/foo",
									Backend: defaultBackend,
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestAnnotations(t *testing.T) {
	ing := buildIngress()

	data := map[string]string{}
	ing.SetAnnotations(data)

	tests := []struct {
		title         string
		ingGrayFlag   bool
		ingGrayCurVer string
		ingGrayNewVer string
		ingGrayIndex  int
	}{
		{"active gray ingress and index 0", true, "1.0", "", 0},
		{"active gray ingress and index 1", true, "1.0", "2.0", 1},
		{"active gray ingress and index 5", true, "1.0", "2.0", 5},
		{"active gray ingress and index 10", true, "", "3.0", 10},
		{"inactive gray ingress and index 0", false, "1.0", "", 0},
		{"inactive gray ingress and index 1", false, "1.0", "2.0", 1},
		{"inactive gray ingress and index 5", false, "1.0", "2.0", 5},
		{"inactive gray ingress and index 10", false, "", "3.0", 10},
		{"active gray ingress and index -1", true, "1.0", "2.0", -1},
		{"inactive gray ingress and index -1", false, "1.0", "2.0", -1},
	}

	for _, test := range tests {
		parser.AnnotationsPrefix = "tengine.taobao.org"
		data[parser.GetAnnotationWithPrefix("ingress-rollout")] = strconv.FormatBool(test.ingGrayFlag)
		data[parser.GetAnnotationWithPrefix("ingress-rollout-current-revision")] = test.ingGrayCurVer
		data[parser.GetAnnotationWithPrefix("ingress-rollout-update-revision")] = test.ingGrayNewVer
		data[parser.GetAnnotationWithPrefix("ingress-rollout-index-id")] = strconv.Itoa(test.ingGrayIndex)

		i, err := NewParser(&resolver.Mock{}).Parse(ing)
		if err != nil {
			t.Errorf("%v: unexpected error: %v", test.title, err)
		}

		u, ok := i.(*Config)
		if !ok {
			t.Errorf("%v: expected an External type", test.title)
		}
		if u.IngGrayFlag != test.ingGrayFlag {
			t.Errorf("%v: IngGrayFlag expected \"%v\" but \"%v\" was returned", test.title, test.ingGrayFlag, u.IngGrayFlag)
		}
		if u.IngGrayCurVer != test.ingGrayCurVer {
			t.Errorf("%v: IngGrayCurVer expected \"%v\" but \"%v\" was returned", test.title, test.ingGrayCurVer, u.IngGrayCurVer)
		}
		if u.IngGrayNewVer != test.ingGrayNewVer {
			t.Errorf("%v: IngGrayNewVer expected \"%v\" but \"%v\" was returned", test.title, test.ingGrayNewVer, u.IngGrayNewVer)
		}
		if u.IngGrayIndex != test.ingGrayIndex {
			t.Errorf("%v: IngGrayIndex expected \"%v\" but \"%v\" was returned", test.title, test.ingGrayIndex, u.IngGrayIndex)
		}
	}
}
