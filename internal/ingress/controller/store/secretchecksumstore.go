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
	"fmt"

	"k8s.io/client-go/tools/cache"
	secretcheckv1 "k8s.io/ingress-nginx/internal/checksum/secret/apis/checksum/v1"
)

// SecretCheckSumStore makes a Store that lists SecretCheckSum
type SecretCheckSumStore struct {
	cache.ThreadSafeStore
}

// NewSecretCheckSumStore creates a new SecretCheckSum store
func NewSecretCheckSumStore() *SecretCheckSumStore {
	return &SecretCheckSumStore{
		cache.NewThreadSafeStore(cache.Indexers{}, cache.Indices{}),
	}
}

// ByKey returns the SecretCheckSum matching key in the local store or an error
func (s SecretCheckSumStore) ByKey(key string) (*secretcheckv1.SecretCheckSum, error) {
	secretCheckSum, exists := s.Get(key)
	if !exists {
		return nil, fmt.Errorf("local SecretCheckSum %v was not found", key)
	}
	return secretCheckSum.(*secretcheckv1.SecretCheckSum), nil
}
