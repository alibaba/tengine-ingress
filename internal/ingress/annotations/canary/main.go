/*
Copyright 2018 The Kubernetes Authors.
Copyright 2022-2023 The Alibaba Authors.

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

package canary

import (
	networking "k8s.io/api/networking/v1"
	"k8s.io/klog"

	"k8s.io/ingress-nginx/internal/ingress/annotations/parser"
	"k8s.io/ingress-nginx/internal/ingress/errors"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
)

const (
	// A list of routing priority for canary ingresses
	CanaryPriorityList = "canary-priority-list"
	// Enable or disable canary ingress
	CanaryFlag = "canary"
	// Canary routing based on header with value 'always'
	CanaryByHeader = "canary-by-header"
	// Canary routing based on header with specific values
	// Format: <header value>[||<header value>]*
	// Default max number of header value is 20
	CanaryByHeaderVal = "canary-by-header-value"
	// Canary routing based on cookie with value 'always'
	CanaryByCookie = "canary-by-cookie"
	// Canary routing based on cookie with specific values
	// Format: <cookie value>[||<cookie value>]*
	// Default max number cookie value is 20
	CanaryByCookieVal = "canary-by-cookie-value"
	// Canary routing based on query with value 'always'
	CanaryByQuery = "canary-by-query"
	// Canary routing based on query with specific values
	// Format: <query value>[||<query value>]*
	// Default max number query value is 20
	CanaryByQueryVal = "canary-by-query-value"
	// Mod divisor
	CanaryModDivisor = "canary-mod-divisor"
	// Mod relational operator
	CanaryModRelationalOpr = "canary-mod-relational-operator"
	// Mod remainder
	CanaryModRemainder = "canary-mod-remainder"
	// Canary weight
	// range: [0, CanaryWeightTotal]
	CanaryWeight = "canary-weight"
	// canary weight total
	// Default range: [100, 10000]
	CanaryWeightTotal = "canary-weight-total"
	// Add header to request based on canary ingress
	// Format: <header name>:<header value>[||<header name>:<header value>]*
	// Default max number header is 2
	// If the header is present on the request, the header and value will be added to the request again.
	CanaryReqAddHeader = "canary-request-add-header"
	// Append header value to request header based on canary ingress
	// Format: <header name>:<header value>[||<header name>:<header value>]*
	// Default max number header is 2
	// If the header is not present on the request, the header will be added to the request.
	CanaryReqAppendHeader = "canary-request-append-header"
	// Add query to request based on canary ingress
	// Format: <query name>=<query value>[&<query name>=<query value>]*
	// Default max number query is 2
	// If the query is present on the request, the query and value will be added to the request again.
	CanaryReqAddQuery = "canary-request-add-query"
	// Add header to response based on canary ingress
	// Format: <header name>:<header value>[||<header name>:<header value>]*
	// Default max number header is 2
	// If the header is present on the request, the header and value will be added to the response again.
	CanaryRespAddHeader = "canary-response-add-header"
	// Append header value to response header based on canary ingress
	// Format: <header name>:<header value>[||<header name>:<header value>]*
	// Default max number header is 2
	// If the header is not present on the request, the header will be added to the response.
	CanaryRespAppendHeader = "canary-response-append-header"
	// Referrer of canary ingress
	CanaryReferrer = "canary-referrer"
)

type canary struct {
	r resolver.Resolver
}

// Config returns the configuration rules for setting up the Canary
type Config struct {
	Enabled          bool
	Weight           int
	WeightTotal      int
	Header           string
	HeaderValue      string
	Cookie           string
	CookieValue      string
	Query            string
	QueryValue       string
	ModDivisor       int
	ModRelationalOpr string
	ModRemainder     int
	ReqAddHeader     string
	ReqAppendHeader  string
	ReqAddQuery      string
	RespAddHeader    string
	RespAppendHeader string
	Priority         string
	Referrer         string
}

// NewParser parses the ingress for canary related annotations
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return canary{r}
}

// Parse parses the annotations contained in the ingress
// rule used to indicate if the canary should be enabled and with what config
func (c canary) Parse(ing *networking.Ingress) (interface{}, error) {
	config := &Config{}
	var err error

	config.Enabled, err = parser.GetBoolAnnotation(CanaryFlag, ing)
	if err != nil {
		config.Enabled = false
	}

	config.Weight, err = parser.GetIntAnnotation(CanaryWeight, ing)
	if err != nil {
		config.Weight = 0
	}

	config.WeightTotal, err = parser.GetIntAnnotation(CanaryWeightTotal, ing)
	if err != nil {
		config.WeightTotal = 100
	}

	config.Header, err = parser.GetStringAnnotation(CanaryByHeader, ing)
	if err != nil {
		config.Header = ""
	}

	config.HeaderValue, err = parser.GetStringAnnotation(CanaryByHeaderVal, ing)
	if err != nil {
		config.HeaderValue = ""
	}

	config.Cookie, err = parser.GetStringAnnotation(CanaryByCookie, ing)
	if err != nil {
		config.Cookie = ""
	}

	config.CookieValue, err = parser.GetStringAnnotation(CanaryByCookieVal, ing)
	if err != nil {
		config.CookieValue = ""
	}

	config.Query, err = parser.GetStringAnnotation(CanaryByQuery, ing)
	if err != nil {
		config.Query = ""
	}

	config.QueryValue, err = parser.GetStringAnnotation(CanaryByQueryVal, ing)
	if err != nil {
		config.QueryValue = ""
	}

	config.ModDivisor, err = parser.GetIntAnnotation(CanaryModDivisor, ing)
	if err != nil {
		config.ModDivisor = 0
	}

	config.ModRelationalOpr, err = parser.GetStringAnnotation(CanaryModRelationalOpr, ing)
	if err != nil {
		config.ModRelationalOpr = ""
	}

	config.ModRemainder, err = parser.GetIntAnnotation(CanaryModRemainder, ing)
	if err != nil {
		config.ModRemainder = 0
	}

	config.ReqAddHeader, err = parser.GetStringAnnotation(CanaryReqAddHeader, ing)
	if err != nil {
		config.ReqAddHeader = ""
	}

	config.ReqAppendHeader, err = parser.GetStringAnnotation(CanaryReqAppendHeader, ing)
	if err != nil {
		config.ReqAppendHeader = ""
	}

	config.ReqAddQuery, err = parser.GetStringAnnotation(CanaryReqAddQuery, ing)
	if err != nil {
		config.ReqAddQuery = ""
	}

	config.RespAddHeader, err = parser.GetStringAnnotation(CanaryRespAddHeader, ing)
	if err != nil {
		config.RespAddHeader = ""
	}

	config.RespAppendHeader, err = parser.GetStringAnnotation(CanaryRespAppendHeader, ing)
	if err != nil {
		config.RespAppendHeader = ""
	}

	config.Referrer, err = parser.GetStringAnnotation(CanaryReferrer, ing)
	if err != nil {
		config.Referrer = ""
	}

	config.Priority, err = parser.GetStringAnnotation(CanaryPriorityList, ing)
	if err != nil {
		config.Priority = ""
	}

	if !config.Enabled &&
		(config.Weight > 0 ||
			len(config.Header) > 0 ||
			len(config.HeaderValue) > 0 ||
			len(config.Cookie) > 0 ||
			len(config.CookieValue) > 0 ||
			len(config.Query) > 0 ||
			len(config.QueryValue) > 0) {
		klog.Warningf("Canary ingress[%v/%v] configured but not enabled, ignored", ing.Namespace, ing.Name)
		return nil, errors.NewInvalidAnnotationConfiguration("canary", "configured but not enabled")
	}

	if config.Enabled && len(config.Referrer) == 0 {
		klog.Warningf("Canary ingress[%v/%v] with empty referrer, ignored", ing.Namespace, ing.Name)
		return nil, errors.NewInvalidAnnotationConfiguration("canary", "referrer is empty")
	}

	return config, nil
}
