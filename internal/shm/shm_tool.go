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

package shm

import (
	"reflect"
	"unsafe"
)

func copySlice2Ptr(b []byte, p uintptr, off int64, size int32) int {
	h := reflect.SliceHeader{}
	h.Cap = int(size)
	h.Len = int(size)
	h.Data = p

	bb := *(*[]byte)(unsafe.Pointer(&h))
	return copy(bb[off:], b)
}

func copyPtr2Slice(p uintptr, b []byte, off int64, size int32) int {
	h := reflect.SliceHeader{}
	h.Cap = int(size)
	h.Len = int(size)
	h.Data = p

	bb := *(*[]byte)(unsafe.Pointer(&h))
	return copy(b, bb[off:size])
}
