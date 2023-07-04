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
	"k8s.io/ingress-nginx/internal/ingress/secannotations/checksum"
	"k8s.io/klog"
	secretcheckv1 "tengine.taobao.org/checksum/secret/apis/checksum/v1"
)

const (
	// Join a md5 string by a separator
	SecretFlag = ","
)

func secretCheck(secrets []*ingress.Secret, secretCheckSums []*secretcheckv1.SecretCheckSum) (bool, error) {
	if len(secretCheckSums) == 0 {
		klog.Infof("Check Secret ID ignored for empty SecretCheckSum")
		return true, nil
	}

	if len(secrets) == 0 {
		klog.Infof("Check Secret ID ignored for empty secrets")
		return false, nil
	}

	secretIDs := make([]string, 0)
	for _, secret := range secrets {
		secretID := getSecretID(secret)
		if secretID != "" {
			secretIDs = append(secretIDs, secretID)
		}
	}

	sort.Strings(secretIDs)
	secretStr := strings.Join(secretIDs, SecretFlag)
	secretData := []byte(secretStr)
	md5str := fmt.Sprintf("%x", md5.Sum(secretData))
	klog.Infof("Check Secret ID: {md5[%v]}", md5str)

	for _, secretCheckSum := range secretCheckSums {
		klog.Infof("Check Secret ID: {md5[%v]} with SecretCheckSum [%v/%v]{checksum[%v], timestamp[%v]}", md5str, secretCheckSum.Namespace, secretCheckSum.Name, secretCheckSum.Spec.Checksum, secretCheckSum.Spec.Timestamp)
		if md5str == secretCheckSum.Spec.Checksum {
			klog.Infof("Check Secret ID: {md5[%v]} is same as the SecretCheckSum [%v/%v]{checksum[%v], timestamp[%v]}", md5str, secretCheckSum.Namespace, secretCheckSum.Name, secretCheckSum.Spec.Checksum, secretCheckSum.Spec.Timestamp)
			return true, nil
		}
	}

	diff := secretDiff(md5str, secretIDs, secretCheckSums)
	return false, errors.New(fmt.Sprintf("Check Secret ID: {md5[%v]} is wrong, diff: {%v}", md5str, diff))
}

func secretDiff(md5str string, secretIDs []string, secretCheckSums []*secretcheckv1.SecretCheckSum) string {
	latestSecretCheckSum := secretCheckSums[0]
	klog.Infof("Check Secret ID: diff {md5[%v]} with SecretCheckSum [%v/%v]{checksum[%v], timestamp[%v]}", md5str, latestSecretCheckSum.Namespace, latestSecretCheckSum.Name, latestSecretCheckSum.Spec.Checksum, latestSecretCheckSum.Spec.Timestamp)
	var diff1, diff2 []string

	// Loop two times:
	// first to find slice1 strings not in slice2 and saved in diff1,
	// second to find slice2 strings not in slice1 and saved in diff2.
	slice1 := secretIDs
	slice2 := latestSecretCheckSum.Spec.Ids
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

	diffResult := fmt.Sprintf("tengine secrets not in SecretCheckSum: <%v> and SecretCheckSum secrets not in tengine: <%v>", strings.Join(diff1, SecretFlag), strings.Join(diff2, SecretFlag))
	return diffResult
}

func getSecretID(secret *ingress.Secret) string {
	secretID := ""
	anns := secret.ParsedAnnotations
	version := anns.CheckSum.SecretVersion

	if version == checksum.DefaultSecretVer {
		klog.Warningf("Secret ID:[%s] has not version info", secret.Name)
		return secretID
	}

	secretNameItems := strings.Split(secret.Name, "-")
	secretItemNum := len(secretNameItems)
	if secretItemNum < 2 {
		klog.Warningf("Secret ID:[%s] name is not valid", secret.Name)
		return secretID
	}

	secretNameID := secretNameItems[secretItemNum-1]
	id, err := strconv.Atoi(secretNameID)
	if err != nil {
		klog.Warningf("Secret ID:[%s] id is not valid", secretNameID)
		return secretID
	}

	if secret.SSLCert == nil {
		klog.Warningf("Secret ID:[%s] has not valid X.509 certificate", secret.Name)
		return secretID
	}

	secretID = fmt.Sprintf("%d-%d-%s", id, version, secret.SSLCert.PemSHA)
	klog.Infof("Secret ID:[%s] is created", secretID)

	return secretID
}
