/*
Copyright 2018 The Kubernetes Authors.
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

package metric

import (
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/ingress-nginx/internal/ingress"
)

// NewDummyCollector returns a dummy metric collector
func NewDummyCollector() Collector {
	return &DummyCollector{}
}

// DummyCollector dummy implementation for mocks in tests
type DummyCollector struct{}

// ConfigSuccess ...
func (dc DummyCollector) ConfigSuccess(uint64, bool) {}

// IncReloadCount ...
func (dc DummyCollector) IncReloadCount() {}

// IncReloadErrorCount ...
func (dc DummyCollector) IncReloadErrorCount() {}

// IncCheckCount ...
func (dc DummyCollector) IncCheckCount(string, string) {}

// IncCheckErrorCount ...
func (dc DummyCollector) IncCheckErrorCount(string, string) {}

// RemoveMetrics ...
func (dc DummyCollector) RemoveMetrics(ingresses, endpoints []string) {}

// Start ...
func (dc DummyCollector) Start() {}

// Stop ...
func (dc DummyCollector) Stop() {}

// SetSSLExpireTime ...
func (dc DummyCollector) SetSSLExpireTime([]*ingress.Server) {}

// SetHosts ...
func (dc DummyCollector) SetHosts(hosts sets.String) {}

// OnStartedLeading indicates the pod is not the current leader
func (dc DummyCollector) OnStartedLeading(electionID string) {}

// OnStoppedLeading indicates the pod is not the current leader
func (dc DummyCollector) OnStoppedLeading(electionID string) {}

// IncIngChecksumCount ...
func (dc DummyCollector) IncIngChecksumCount() {}

// IncIngChecksumErrorCount ...
func (dc DummyCollector) IncIngChecksumErrorCount() {}

// ClearIngChecksumErrorCount ...
func (dc DummyCollector) ClearIngChecksumErrorCount() {}

// IncSSLCertVerifyFailCount ...
func (dc DummyCollector) IncSSLCertVerifyFailCount() {}

// IncIngReferInvalidCount ...
func (dc DummyCollector) IncIngReferInvalidCount() {}

// IncCanaryReferInvalidCount ...
func (dc DummyCollector) IncCanaryReferInvalidCount() {}

// IncCanaryNumLimitExCount ...
func (dc DummyCollector) IncCanaryNumLimitExCount() {}

// IncSecretChecksumCount ...
func (dc DummyCollector) IncSecretChecksumCount() {}

// IncSecretChecksumErrorCount ...
func (dc DummyCollector) IncSecretChecksumErrorCount() {}

// ClearSecretChecksumErrorCount ...
func (dc DummyCollector) ClearSecretChecksumErrorCount() {}
