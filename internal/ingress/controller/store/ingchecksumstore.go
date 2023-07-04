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
	ingcheckv1 "tengine.taobao.org/checksum/ingress/apis/checksum/v1"
)

// IngressCheckSumStore makes a Store that lists IngressCheckSum
type IngressCheckSumStore struct {
	cache.ThreadSafeStore
}

// NewIngressCheckSumStore creates a new IngressCheckSum store
func NewIngressCheckSumStore() *IngressCheckSumStore {
	return &IngressCheckSumStore{
		cache.NewThreadSafeStore(cache.Indexers{}, cache.Indices{}),
	}
}

// ByKey returns the IngressCheckSum matching key in the local store or an error
func (s IngressCheckSumStore) ByKey(key string) (*ingcheckv1.IngressCheckSum, error) {
	ingCheckSum, exists := s.Get(key)
	if !exists {
		return nil, fmt.Errorf("local IngressCheckSum %v was not found", key)
	}
	return ingCheckSum.(*ingcheckv1.IngressCheckSum), nil
}
