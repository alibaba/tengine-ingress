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
	ingcheckv1 "tengine.taobao.org/checksum/ingress/apis/checksum/v1"
)

// IngressCheckSumLister makes a Store that lists IngressCheckSum
type IngressCheckSumLister struct {
	cache.Store
}

// ByKey returns the IngressCheckSum matching key in the local store or an error
func (il IngressCheckSumLister) ByKey(key string) (*ingcheckv1.IngressCheckSum, error) {
	i, exists, err := il.GetByKey(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, NotExistsError(key)
	}
	return i.(*ingcheckv1.IngressCheckSum), nil
}
