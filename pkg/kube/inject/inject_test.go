// Copyright 2018 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package inject

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"

	meshapi "istio.io/api/mesh/v1alpha1"

	"istio.io/istio/pilot/test/util"
	"istio.io/istio/pkg/config/mesh"

	corev1 "k8s.io/api/core/v1"
)

const (
	// This is the hub to expect in
	// platform/kube/inject/testdata/frontend.yaml.injected and the
	// other .injected "want" YAMLs
	unitTestHub = "docker.io/istio"

	// Tag name should be kept in sync with value in
	// platform/kube/inject/refresh.sh
	unitTestTag = "unittest"

	statusReplacement = "sidecar.istio.io/status: '{\"version\":\"\","
)

var (
	statusPattern = regexp.MustCompile("sidecar.istio.io/status: '{\"version\":\"([0-9a-f]+)\",")
)

// InitImageName returns the fully qualified image name for the istio
// init image given a docker hub and tag and debug flag
// This is used for testing only
func InitImageName(hub string, tag string) string {
	return hub + "/proxy_init:" + tag
}

// ProxyImageName returns the fully qualified image name for the istio
// proxy image given a docker hub and tag and whether to use debug or not.
// This is used for testing
func ProxyImageName(hub string, tag string) string {
	// Allow overriding the proxy image.
	return hub + "/proxyv2:" + tag
}

func TestImageName(t *testing.T) {
	want := "docker.io/istio/proxy_init:latest"
	if got := InitImageName("docker.io/istio", "latest"); got != want {
		t.Errorf("InitImageName() failed: got %q want %q", got, want)
	}
	want = "docker.io/istio/proxyv2:latest"
	if got := ProxyImageName("docker.io/istio", "latest"); got != want {
		t.Errorf("ProxyImageName() failed: got %q want %q", got, want)
	}
}

func TestIntoResourceFile(t *testing.T) {
	cases := []struct {
		in                           string
		want                         string
		imagePullPolicy              string
		duration                     time.Duration
		includeIPRanges              string
		excludeIPRanges              string
		includeInboundPorts          string
		excludeInboundPorts          string
		kubevirtInterfaces           string
		statusPort                   int
		readinessInitialDelaySeconds uint32
		readinessPeriodSeconds       uint32
		readinessFailureThreshold    uint32
		enableAuth                   bool
		enableCoreDump               bool
		privileged                   bool
		tproxy                       bool
		podDNSSearchNamespaces       []string
		enableCni                    bool
	}{
		//"testdata/hello.yaml" is tested in http_test.go (with debug)
		{
			in:                           "hello.yaml",
			want:                         "hello.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		// verify cni
		{
			in:                           "hello.yaml",
			want:                         "hello.yaml.cni.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
			enableCni:                    true,
		},
		//verifies that the sidecar will not be injected again for an injected yaml
		{
			in:                           "hello.yaml.injected",
			want:                         "hello.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "hello-mtls-not-ready.yaml",
			want:                         "hello-mtls-not-ready.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "hello-namespace.yaml",
			want:                         "hello-namespace.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "hello-proxy-override.yaml",
			want:                         "hello-proxy-override.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:     "hello.yaml",
			want:   "hello-tproxy.yaml.injected",
			tproxy: true,
		},
		{
			in:                           "hello.yaml",
			want:                         "hello-config-map-name.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "frontend.yaml",
			want:                         "frontend.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "hello-service.yaml",
			want:                         "hello-service.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "hello-multi.yaml",
			want:                         "hello-multi.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "hello.yaml",
			want:                         "hello-always.yaml.injected",
			imagePullPolicy:              "Always",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "hello.yaml",
			want:                         "hello-never.yaml.injected",
			imagePullPolicy:              "Never",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "hello-ignore.yaml",
			want:                         "hello-ignore.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "multi-init.yaml",
			want:                         "multi-init.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "statefulset.yaml",
			want:                         "statefulset.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "enable-core-dump.yaml",
			want:                         "enable-core-dump.yaml.injected",
			enableCoreDump:               true,
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "enable-core-dump-annotation.yaml",
			want:                         "enable-core-dump-annotation.yaml.injected",
			enableCoreDump:               false,
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "auth.yaml",
			want:                         "auth.yaml.injected",
			enableAuth:                   true,
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "auth.non-default-service-account.yaml",
			want:                         "auth.non-default-service-account.yaml.injected",
			enableAuth:                   true,
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "auth.yaml",
			want:                         "auth.cert-dir.yaml.injected",
			enableAuth:                   true,
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "daemonset.yaml",
			want:                         "daemonset.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "job.yaml",
			want:                         "job.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "replicaset.yaml",
			want:                         "replicaset.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "replicationcontroller.yaml",
			want:                         "replicationcontroller.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "cronjob.yaml",
			want:                         "cronjob.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "pod.yaml",
			want:                         "pod.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "hello-host-network.yaml",
			want:                         "hello-host-network.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "list.yaml",
			want:                         "list.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "list-frontend.yaml",
			want:                         "list-frontend.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "deploymentconfig.yaml",
			want:                         "deploymentconfig.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "deploymentconfig-multi.yaml",
			want:                         "deploymentconfig-multi.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			in:                           "format-duration.yaml",
			want:                         "format-duration.yaml.injected",
			duration:                     42 * time.Second,
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			// Verifies that parameters are applied properly when no annotations are provided.
			in:                  "traffic-params.yaml",
			want:                "traffic-params.yaml.injected",
			includeIPRanges:     "127.0.0.1/24,10.96.0.1/24",
			excludeIPRanges:     "10.96.0.2/24,10.96.0.3/24",
			includeInboundPorts: "1,2,3",
			excludeInboundPorts: "4,5,6",
			statusPort:          0,
		},
		{
			// Verifies that empty include lists are applied properly from parameters.
			in:                           "traffic-params-empty-includes.yaml",
			want:                         "traffic-params-empty-includes.yaml.injected",
			includeIPRanges:              "",
			excludeIPRanges:              "",
			kubevirtInterfaces:           "",
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			// Verifies that annotation values are applied properly. This also tests that annotation values
			// override params when specified.
			in:                           "traffic-annotations.yaml",
			want:                         "traffic-annotations.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			// Verifies that the wildcard character "*" behaves properly when used in annotations.
			in:                           "traffic-annotations-wildcards.yaml",
			want:                         "traffic-annotations-wildcards.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			// Verifies that the wildcard character "*" behaves properly when used in annotations.
			in:                           "traffic-annotations-empty-includes.yaml",
			want:                         "traffic-annotations-empty-includes.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			// Verifies that pods can have multiple containers
			in:                           "multi-container.yaml",
			want:                         "multi-container.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			// Verifies that the status params behave properly.
			in:                           "status_params.yaml",
			want:                         "status_params.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			kubevirtInterfaces:           DefaultkubevirtInterfaces,
			statusPort:                   123,
			readinessInitialDelaySeconds: 100,
			readinessPeriodSeconds:       200,
			readinessFailureThreshold:    300,
		},
		{
			// Verifies that the status annotations override the params.
			in:                           "status_annotations.yaml",
			want:                         "status_annotations.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			// Verifies that the kubevirtInterfaces list are applied properly from parameters..
			in:                           "kubevirtInterfaces.yaml",
			want:                         "kubevirtInterfaces.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			kubevirtInterfaces:           "net1",
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			// Verifies that the kubevirtInterfaces list are applied properly from parameters..
			in:                           "kubevirtInterfaces_list.yaml",
			want:                         "kubevirtInterfaces_list.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			kubevirtInterfaces:           "net1,net2",
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
		},
		{
			// Verifies that global.podDNSSearchNamespaces are applied properly
			in:                           "hello.yaml",
			want:                         "hello-template-in-values.yaml.injected",
			includeIPRanges:              DefaultIncludeIPRanges,
			includeInboundPorts:          DefaultIncludeInboundPorts,
			kubevirtInterfaces:           "net1,net2",
			statusPort:                   DefaultStatusPort,
			readinessInitialDelaySeconds: DefaultReadinessInitialDelaySeconds,
			readinessPeriodSeconds:       DefaultReadinessPeriodSeconds,
			readinessFailureThreshold:    DefaultReadinessFailureThreshold,
			podDNSSearchNamespaces: []string{
				"global",
				"{{ valueOrDefault .DeploymentMeta.Namespace \"default\" }}.global",
			},
		},
	}

	for i, c := range cases {
		testName := fmt.Sprintf("[%02d] %s", i, c.want)
		t.Run(testName, func(t *testing.T) {
			m := mesh.DefaultMeshConfig()
			if c.duration != 0 {
				m.DefaultConfig.DrainDuration = types.DurationProto(c.duration)
				m.DefaultConfig.ParentShutdownDuration = types.DurationProto(c.duration)
				m.DefaultConfig.ConnectTimeout = types.DurationProto(c.duration)
			}
			if c.tproxy {
				m.DefaultConfig.InterceptionMode = meshapi.ProxyConfig_TPROXY
			} else {
				m.DefaultConfig.InterceptionMode = meshapi.ProxyConfig_REDIRECT
			}

			params := &Params{
				InitImage:                    InitImageName(unitTestHub, unitTestTag),
				ProxyImage:                   ProxyImageName(unitTestHub, unitTestTag),
				ImagePullPolicy:              "IfNotPresent",
				SDSEnabled:                   false,
				Verbosity:                    DefaultVerbosity,
				SidecarProxyUID:              DefaultSidecarProxyUID,
				Version:                      "12345678",
				EnableCoreDump:               c.enableCoreDump,
				Privileged:                   c.privileged,
				Mesh:                         &m,
				IncludeIPRanges:              c.includeIPRanges,
				ExcludeIPRanges:              c.excludeIPRanges,
				IncludeInboundPorts:          c.includeInboundPorts,
				ExcludeInboundPorts:          c.excludeInboundPorts,
				KubevirtInterfaces:           c.kubevirtInterfaces,
				StatusPort:                   c.statusPort,
				ReadinessInitialDelaySeconds: c.readinessInitialDelaySeconds,
				ReadinessPeriodSeconds:       c.readinessPeriodSeconds,
				ReadinessFailureThreshold:    c.readinessFailureThreshold,
				RewriteAppHTTPProbe:          false,
				PodDNSSearchNamespaces:       c.podDNSSearchNamespaces,
				EnableCni:                    c.enableCni,
			}
			if c.imagePullPolicy != "" {
				params.ImagePullPolicy = c.imagePullPolicy
			}
			sidecarTemplate := loadSidecarTemplate(t)
			valuesConfig := getValues(params, t)
			inputFilePath := "testdata/inject/" + c.in
			wantFilePath := "testdata/inject/" + c.want
			in, err := os.Open(inputFilePath)
			if err != nil {
				t.Fatalf("Failed to open %q: %v", inputFilePath, err)
			}
			defer func() { _ = in.Close() }()
			var got bytes.Buffer
			if err = IntoResourceFile(sidecarTemplate, valuesConfig, &m, in, &got); err != nil {
				t.Fatalf("IntoResourceFile(%v) returned an error: %v", inputFilePath, err)
			}

			// The version string is a maintenance pain for this test. Strip the version string before comparing.
			gotBytes := got.Bytes()
			wantedBytes := util.ReadGoldenFile(gotBytes, wantFilePath, t)

			wantBytes := stripVersion(wantedBytes)
			gotBytes = stripVersion(gotBytes)

			util.CompareBytes(gotBytes, wantBytes, wantFilePath, t)

			if util.Refresh() {
				util.RefreshGoldenFile(gotBytes, wantFilePath, t)
			}
		})
	}
}

// TestRewriteAppProbe tests the feature for pilot agent to take over app health check traffic.
func TestRewriteAppProbe(t *testing.T) {
	cases := []struct {
		in                  string
		rewriteAppHTTPProbe bool
		want                string
	}{
		{
			in:                  "hello-probes.yaml",
			rewriteAppHTTPProbe: true,
			want:                "hello-probes.yaml.injected",
		},
		{
			in:                  "hello-readiness.yaml",
			rewriteAppHTTPProbe: true,
			want:                "hello-readiness.yaml.injected",
		},
		{
			in:                  "named_port.yaml",
			rewriteAppHTTPProbe: true,
			want:                "named_port.yaml.injected",
		},
		{
			in:                  "one_container.yaml",
			rewriteAppHTTPProbe: true,
			want:                "one_container.yaml.injected",
		},
		{
			in:                  "two_container.yaml",
			rewriteAppHTTPProbe: true,
			want:                "two_container.yaml.injected",
		},
		{
			in:                  "ready_only.yaml",
			rewriteAppHTTPProbe: true,
			want:                "ready_only.yaml.injected",
		},
		{
			in:                  "https-probes.yaml",
			rewriteAppHTTPProbe: true,
			want:                "https-probes.yaml.injected",
		},
		{
			in:                  "hello-probes-with-flag-set-in-annotation.yaml",
			rewriteAppHTTPProbe: false,
			want:                "hello-probes-with-flag-set-in-annotation.yaml.injected",
		},
		{
			in:                  "hello-probes-with-flag-unset-in-annotation.yaml",
			rewriteAppHTTPProbe: true,
			want:                "hello-probes-with-flag-unset-in-annotation.yaml.injected",
		},
		{
			in:                  "ready_live.yaml",
			rewriteAppHTTPProbe: true,
			want:                "ready_live.yaml.injected",
		},
		// TODO(incfly): add more test case covering different -statusPort=123, --statusPort=123
		// No statusport, --statusPort 123.
	}

	for i, c := range cases {
		testName := fmt.Sprintf("[%02d] %s", i, c.want)
		t.Run(testName, func(t *testing.T) {
			m := mesh.DefaultMeshConfig()
			params := &Params{
				InitImage:                    InitImageName(unitTestHub, unitTestTag),
				ProxyImage:                   ProxyImageName(unitTestHub, unitTestTag),
				ImagePullPolicy:              "IfNotPresent",
				SidecarProxyUID:              DefaultSidecarProxyUID,
				Version:                      "12345678",
				StatusPort:                   DefaultStatusPort,
				ReadinessInitialDelaySeconds: DefaultReadinessPeriodSeconds,
				ReadinessPeriodSeconds:       DefaultReadinessFailureThreshold,
				ReadinessFailureThreshold:    DefaultReadinessFailureThreshold,
				RewriteAppHTTPProbe:          c.rewriteAppHTTPProbe,
			}
			sidecarTemplate := loadSidecarTemplate(t)
			valuesConfig := getValues(params, t)
			inputFilePath := "testdata/inject/app_probe/" + c.in
			wantFilePath := "testdata/inject/app_probe/" + c.want
			in, err := os.Open(inputFilePath)
			if err != nil {
				t.Fatalf("Failed to open %q: %v", inputFilePath, err)
			}
			defer func() { _ = in.Close() }()
			var got bytes.Buffer
			if err = IntoResourceFile(sidecarTemplate, valuesConfig, &m, in, &got); err != nil {
				t.Fatalf("IntoResourceFile(%v) returned an error: %v", inputFilePath, err)
			}

			// The version string is a maintenance pain for this test. Strip the version string before comparing.
			gotBytes := got.Bytes()
			wantedBytes := util.ReadGoldenFile(gotBytes, wantFilePath, t)

			wantBytes := stripVersion(wantedBytes)
			gotBytes = stripVersion(gotBytes)

			util.CompareBytes(gotBytes, wantBytes, wantFilePath, t)
		})
	}
}

func stripVersion(yaml []byte) []byte {
	return statusPattern.ReplaceAllLiteral(yaml, []byte(statusReplacement))
}

func TestInvalidParams(t *testing.T) {
	cases := []struct {
		annotation    string
		paramModifier func(p *Params)
	}{
		{
			annotation: "includeipranges",
			paramModifier: func(p *Params) {
				p.IncludeIPRanges = "bad"
			},
		},
		{
			annotation: "excludeipranges",
			paramModifier: func(p *Params) {
				p.ExcludeIPRanges = "*"
			},
		},
		{
			annotation: "includeinboundports",
			paramModifier: func(p *Params) {
				p.IncludeInboundPorts = "bad"
			},
		},
		{
			annotation: "excludeinboundports",
			paramModifier: func(p *Params) {
				p.ExcludeInboundPorts = "*"
			},
		},
	}

	for _, c := range cases {
		t.Run(c.annotation, func(t *testing.T) {
			params := newTestParams()
			c.paramModifier(params)

			if err := params.Validate(); err == nil {
				t.Fatalf("expected error")
			} else if !strings.Contains(strings.ToLower(err.Error()), c.annotation) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestInvalidAnnotations(t *testing.T) {
	cases := []struct {
		annotation string
		in         string
	}{
		{
			annotation: "includeipranges",
			in:         "traffic-annotations-bad-includeipranges.yaml",
		},
		{
			annotation: "excludeipranges",
			in:         "traffic-annotations-bad-excludeipranges.yaml",
		},
		{
			annotation: "includeinboundports",
			in:         "traffic-annotations-bad-includeinboundports.yaml",
		},
		{
			annotation: "excludeinboundports",
			in:         "traffic-annotations-bad-excludeinboundports.yaml",
		},
		{
			annotation: "excludeoutboundports",
			in:         "traffic-annotations-bad-excludeoutboundports.yaml",
		},
	}

	for _, c := range cases {
		t.Run(c.annotation, func(t *testing.T) {
			params := newTestParams()
			sidecarTemplate := loadSidecarTemplate(t)
			valuesConfig := getValues(params, t)
			inputFilePath := "testdata/inject/" + c.in
			in, err := os.Open(inputFilePath)
			if err != nil {
				t.Fatalf("Failed to open %q: %v", inputFilePath, err)
			}
			defer func() { _ = in.Close() }()
			var got bytes.Buffer
			if err = IntoResourceFile(sidecarTemplate, valuesConfig, params.Mesh, in, &got); err == nil {
				t.Fatalf("expected error")
			} else if !strings.Contains(strings.ToLower(err.Error()), c.annotation) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSkipUDPPorts(t *testing.T) {
	cases := []struct {
		c     corev1.Container
		ports []string
	}{
		{
			c: corev1.Container{
				Ports: []corev1.ContainerPort{},
			},
		},
		{
			c: corev1.Container{
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 80,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						ContainerPort: 8080,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			},
			ports: []string{"80", "8080"},
		},
		{
			c: corev1.Container{
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 53,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						ContainerPort: 53,
						Protocol:      corev1.ProtocolUDP,
					},
				},
			},
			ports: []string{"53"},
		},
		{
			c: corev1.Container{
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 80,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						ContainerPort: 53,
						Protocol:      corev1.ProtocolUDP,
					},
				},
			},
			ports: []string{"80"},
		},
		{
			c: corev1.Container{
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 53,
						Protocol:      corev1.ProtocolUDP,
					},
				},
			},
		},
	}
	for i := range cases {
		expectPorts := cases[i].ports
		ports := getPortsForContainer(cases[i].c)
		if len(ports) != len(expectPorts) {
			t.Fatalf("unexpect ports result for case %d", i)
		}
		for j := 0; j < len(ports); j++ {
			if ports[j] != expectPorts[j] {
				t.Fatalf("unexpect ports result for case %d: expect %v, got %v", i, expectPorts, ports)
			}
		}
	}
}

func newTestParams() *Params {
	m := mesh.DefaultMeshConfig()
	return &Params{
		InitImage:           InitImageName(unitTestHub, unitTestTag),
		ProxyImage:          ProxyImageName(unitTestHub, unitTestTag),
		ImagePullPolicy:     "IfNotPresent",
		SDSEnabled:          false,
		Verbosity:           DefaultVerbosity,
		SidecarProxyUID:     DefaultSidecarProxyUID,
		Version:             "12345678",
		EnableCoreDump:      false,
		Mesh:                &m,
		DebugMode:           false,
		IncludeIPRanges:     DefaultIncludeIPRanges,
		ExcludeIPRanges:     "",
		IncludeInboundPorts: DefaultIncludeInboundPorts,
		ExcludeInboundPorts: "",
		EnableCni:           false,
	}
}
