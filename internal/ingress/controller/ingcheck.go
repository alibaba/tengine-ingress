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

package controller

import (
	"crypto/md5"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"k8s.io/ingress-nginx/internal/ingress"
	"k8s.io/ingress-nginx/internal/ingress/annotations/checksum"
	"k8s.io/klog"
	ingcheckv1 "tengine.taobao.org/checksum/ingress/apis/checksum/v1"
)

const (
	// Join a md5 string by a separator
	IngFlag = ","
)

func ingCheck(ingresses []*ingress.Ingress, ingCheckSums []*ingcheckv1.IngressCheckSum) (bool, error) {
	if len(ingCheckSums) == 0 {
		klog.Infof("Check Ingress ID ignored for empty IngressCheckSum")
		return true, nil
	}

	if len(ingresses) == 0 {
		klog.Infof("Check Ingress ID ignored for empty ingresses")
		return false, nil
	}

	ingIDs := make([]string, 0)

	for _, ing := range ingresses {
		ingID := getIngID(ing)
		if ingID != "" {
			ingIDs = append(ingIDs, ingID)
		}
	}

	sort.Strings(ingIDs)
	ingStr := strings.Join(ingIDs, IngFlag)
	ingData := []byte(ingStr)
	md5str := fmt.Sprintf("%x", md5.Sum(ingData))
	klog.Infof("Check Ingress ID: {md5[%v]}", md5str)

	for _, ingCheckSum := range ingCheckSums {
		klog.Infof("Check Ingress ID: {md5[%v]} with IngressCheckSum [%v/%v]{checksum[%v], timestamp[%v]}", md5str, ingCheckSum.Namespace, ingCheckSum.Name, ingCheckSum.Spec.Checksum, ingCheckSum.Spec.Timestamp)
		if md5str == ingCheckSum.Spec.Checksum {
			klog.Infof("Check Ingress ID: {md5[%v]} is same as the IngressCheckSum [%v/%v]{checksum[%v], timestamp[%v]}", md5str, ingCheckSum.Namespace, ingCheckSum.Name, ingCheckSum.Spec.Checksum, ingCheckSum.Spec.Timestamp)
			return true, nil
		}
	}

	diff := ingDiff(md5str, ingIDs, ingCheckSums)
	return false, errors.New(fmt.Sprintf("Check Ingress ID: {md5[%v]} is wrong, diff: {%v}", md5str, diff))
}

func ingDiff(md5str string, ingIDs []string, ingCheckSums []*ingcheckv1.IngressCheckSum) string {
	latestIngCheckSum := ingCheckSums[0]
	klog.Infof("Check Ingress ID: diff {md5[%v]} with IngressCheckSum [%v/%v]{checksum[%v], timestamp[%v]}", md5str, latestIngCheckSum.Namespace, latestIngCheckSum.Name, latestIngCheckSum.Spec.Checksum, latestIngCheckSum.Spec.Timestamp)
	var diff1, diff2 []string

	// Loop two times:
	// first to find slice1 strings not in slice2 and saved in diff1,
	// second to find slice2 strings not in slice1 and saved in diff2.
	slice1 := ingIDs
	slice2 := latestIngCheckSum.Spec.Ids
	for i := 0; i < 2; i++ {
		for _, s1 := range slice1 {
			found := false
			for _, s2 := range slice2 {
				if s1 == s2 {
					found = true
					break
				}
			}
			// String not found. We add it to return slice
			if !found {
				if i == 0 {
					diff1 = append(diff1, s1)
				} else {
					diff2 = append(diff2, s1)
				}

			}
		}
		// Swap the slices, only if it was the first loop
		if i == 0 {
			slice1, slice2 = slice2, slice1
		}
	}

	diffResult := fmt.Sprintf("tengine ings not in IngressCheckSum: <%v> and IngressCheckSum ings not in tengine: <%v>", strings.Join(diff1, IngFlag), strings.Join(diff2, IngFlag))
	return diffResult
}

func getIngID(ing *ingress.Ingress) string {
	ingID := ""
	anns := ing.ParsedAnnotations
	version := anns.CheckSum.IngVersion

	if version == checksum.DefaultIngVer {
		klog.Warningf("Ingress ID:[%s] has not version info", ing.Name)
		return ingID
	}

	ingNameItems := strings.Split(ing.Name, "-")
	ingItemNum := len(ingNameItems)
	// cluster-host-ingressId
	if ingItemNum < 2 {
		klog.Warningf("Ingress ID:[%s] name is not valid", ing.Name)
		return ingID
	}

	ingNameID := ingNameItems[ingItemNum-1]
	id, err := strconv.Atoi(ingNameID)
	if err != nil {
		klog.Warningf("Ingress ID:[%s] id is not valid", ingNameID)
		return ingID
	}

	ingID = fmt.Sprintf("%d-%d", id, version)
	klog.Infof("Ingress ID:[%s] is created", ingID)

	return ingID
}
