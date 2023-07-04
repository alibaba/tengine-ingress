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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Describes md5 info of all the secret
type SecretCheckSum struct {
	metav1.TypeMeta `json:",inline"`
	// `metadata` is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	// `spec` is the specification of the desired behavior of a SecretCheckSum.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status
	// +optional
	Spec SecretCheckSumSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SecretCheckSumList is a list of SecretCheckSum objects.
type SecretCheckSumList struct {
	metav1.TypeMeta `json:",inline"`
	// `metadata` is the standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// `items` is a list of SecretCheckSum.
	// +listType=set
	Items []SecretCheckSum `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// SecretCheckSumSpec describes how the SecretCheckSum's specification looks like.
type SecretCheckSumSpec struct {
	// `Timestamp` is the time when the md5 of all the secret was calculated.
	Timestamp metav1.Time `json:"timestamp" protobuf:"bytes,1,opt,name=timestamp"`
	// `Checksum` is the md5 of all the secret.
	// +optional
	Checksum string `json:"checksum,omitempty" protobuf:"bytes,2,opt,name=checksum"`
	// `ids` describes which id will match this secret.
	// +listType=set
	// +optional
	Ids []string `json:"ids,omitempty" protobuf:"bytes,3,rep,name=ids"`
}
