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
	"fmt"
	"os/exec"
	"strconv"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/stretchr/testify/assert"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
	"k8s.io/ingress-nginx/internal/ingress/secannotations/parser"
	"k8s.io/ingress-tengine/test/e2e/framework"
)

func buildSecret(username, password, name, namespace string) *apiv1.Secret {
	out, err := exec.Command("openssl", "passwd", "-crypt", password).CombinedOutput()
	assert.Nil(ginkgo.GinkgoT(), err, "creating password")

	encpass := fmt.Sprintf("%v:%s\n", username, out)

	return &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:                       name,
			Namespace:                  namespace,
			DeletionGracePeriodSeconds: framework.NewInt64(1),
		},
		Data: map[string][]byte{
			"auth": []byte(encpass),
		},
		Type: apiv1.SecretTypeOpaque,
	}
}

func TestAnnotations(t *testing.T) {
	ing := buildSecret()

	data := map[string]string{}
	ing.SetAnnotations(data)

	tests := []struct {
		title         string
		secretVersion int
	}{
		{"secret with version -1", -1},
		{"secret with version 0", 0},
		{"secret with version 111", 111},
	}

	for _, test := range tests {
		data[parser.GetAnnotationWithPrefix("version")] = strconv.Itoa(test.secretVersion)

		i, err := NewParser(&resolver.Mock{}).Parse(ing)
		if err != nil {
			t.Errorf("%v: unexpected error: %v", test.title, err)
		}

		u, ok := i.(*Config)
		if !ok {
			t.Errorf("%v: expected an External type", test.title)
		}
		if u.SecretVersion != test.secretVersion {
			t.Errorf("%v: IngGrayIndex expected \"%v\" but \"%v\" was returned", test.title, test.secretVersion, u.SecretVersion)
		}
	}
}
