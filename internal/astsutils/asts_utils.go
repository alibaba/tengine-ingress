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
	"regexp"
	"strconv"

	"k8s.io/ingress-nginx/internal/k8s"
)

var statefulPodRegex = regexp.MustCompile("(.*)-([0-9]+)$")

func GetPodOrdinal(podInfo *k8s.PodInfo) int32 {
	var ordinal int32 = -1
	if podInfo == nil {
		return ordinal
	}

	subMatches := statefulPodRegex.FindStringSubmatch(podInfo.Name)
	if len(subMatches) < 3 {
		return ordinal
	}

	if i, err := strconv.ParseInt(subMatches[2], 10, 32); err == nil {
		ordinal = int32(i)
	}
	return ordinal
}
