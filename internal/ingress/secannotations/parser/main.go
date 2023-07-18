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

package parser

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	apiv1 "k8s.io/api/core/v1"

	"k8s.io/ingress-nginx/internal/ingress/errors"
)

var (
	// AnnotationsPrefix defines the common prefix for the Secret
	AnnotationsPrefix = "nginx.ingress.kubernetes.io"
)

// SecretAnnotation has a method to parse annotations located in Secret
type SecretAnnotation interface {
	Parse(secret *apiv1.Secret) (interface{}, error)
}

type secretAnnotations map[string]string

func (a secretAnnotations) parseBool(name string) (bool, error) {
	val, ok := a[name]
	if ok {
		b, err := strconv.ParseBool(val)
		if err != nil {
			return false, errors.NewInvalidAnnotationContent(name, val)
		}
		return b, nil
	}
	return false, errors.ErrMissingAnnotations
}

func (a secretAnnotations) parseString(name string) (string, error) {
	val, ok := a[name]
	if ok {
		s := normalizeString(val)
		if len(s) == 0 {
			return "", errors.NewInvalidAnnotationContent(name, val)
		}

		return s, nil
	}
	return "", errors.ErrMissingAnnotations
}

func (a secretAnnotations) parseInt(name string) (int, error) {
	val, ok := a[name]
	if ok {
		i, err := strconv.Atoi(val)
		if err != nil {
			return 0, errors.NewInvalidAnnotationContent(name, val)
		}
		return i, nil
	}
	return 0, errors.ErrMissingAnnotations
}

func checkAnnotation(name string, secret *apiv1.Secret) error {
	if secret == nil || len(secret.GetAnnotations()) == 0 {
		return errors.ErrMissingAnnotations
	}
	if name == "" {
		return errors.ErrInvalidAnnotationName
	}

	return nil
}

// GetBoolAnnotation extracts a boolean from an Secret annotation
func GetBoolAnnotation(name string, secret *apiv1.Secret) (bool, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, secret)
	if err != nil {
		return false, err
	}
	return secretAnnotations(secret.GetAnnotations()).parseBool(v)
}

// GetStringAnnotation extracts a string from an Secret annotation
func GetStringAnnotation(name string, secret *apiv1.Secret) (string, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, secret)
	if err != nil {
		return "", err
	}

	return secretAnnotations(secret.GetAnnotations()).parseString(v)
}

// GetIntAnnotation extracts an int from an Secret annotation
func GetIntAnnotation(name string, secret *apiv1.Secret) (int, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, secret)
	if err != nil {
		return 0, err
	}
	return secretAnnotations(secret.GetAnnotations()).parseInt(v)
}

// GetAnnotationWithPrefix returns the prefix of Secret annotations
func GetAnnotationWithPrefix(suffix string) string {
	return fmt.Sprintf("%v/%v", AnnotationsPrefix, suffix)
}

func normalizeString(input string) string {
	trimmedContent := []string{}
	for _, line := range strings.Split(input, "\n") {
		trimmedContent = append(trimmedContent, strings.TrimSpace(line))
	}

	return strings.Join(trimmedContent, "\n")
}

// StringToURL parses the provided string into URL and returns error
// message in case of failure
func StringToURL(input string) (*url.URL, error) {
	parsedURL, err := url.Parse(input)
	if err != nil {
		return nil, fmt.Errorf("%v is not a valid URL: %v", input, err)
	}

	if parsedURL.Scheme == "" {
		return nil, fmt.Errorf("url scheme is empty")
	} else if parsedURL.Host == "" {
		return nil, fmt.Errorf("url host is empty")
	} else if strings.Contains(parsedURL.Host, "..") {
		return nil, fmt.Errorf("invalid url host")
	}

	return parsedURL, nil
}
