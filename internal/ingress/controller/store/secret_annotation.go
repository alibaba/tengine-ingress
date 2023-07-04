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

package store

import (
	"k8s.io/client-go/tools/cache"
	"k8s.io/ingress-nginx/internal/ingress"
)

// SecretWithAnnotationsLister makes a Store that lists Secret with annotations already parsed
type SecretWithAnnotationsLister struct {
	cache.Store
}

// ByKey returns the Secret with annotations matching key in the local store or an error
func (il SecretWithAnnotationsLister) ByKey(key string) (*ingress.Secret, error) {
	i, exists, err := il.GetByKey(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, NotExistsError(key)
	}
	return i.(*ingress.Secret), nil
}
