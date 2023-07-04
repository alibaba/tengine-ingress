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

package astsutils

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-nginx/internal/k8s"
)

func TestAstsPodFirst(t *testing.T) {
	pod := &k8s.PodInfo{
		Name:      "tengine-ingress-0",
		Namespace: v1.NamespaceDefault,
	}

	i := GetPodOrdinal(pod)
	if i != 0 {
		t.Errorf("Pod index returned %v, but expected 0", i)
	}
}

func TestAstsPodLast(t *testing.T) {
	pod := &k8s.PodInfo{
		Name:      "tengine-ingress-5000",
		Namespace: v1.NamespaceDefault,
	}

	i := GetPodOrdinal(pod)
	if i != 5000 {
		t.Errorf("Pod index returned %v, but expected 5000", i)
	}
}

func TestNormalPod(t *testing.T) {
	pod := &k8s.PodInfo{
		Name:      "tengine-ingress",
		Namespace: v1.NamespaceDefault,
	}

	i := GetPodOrdinal(pod)
	if i != -1 {
		t.Errorf("Pod index returned %v, but expected -1", i)
	}
}

func TestEmptyPod(t *testing.T) {
	i := GetPodOrdinal(nil)
	if i != -1 {
		t.Errorf("Pod index returned %v, but expected -1", i)
	}
}
