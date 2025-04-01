/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog/v2"

	corev1 "k8s.io/api/core/v1"
	//discoveryv1 "k8s.io/api/discovery/v1"
	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"

	"k8s.io/ingress-nginx/internal/k8s"
	"k8s.io/ingress-nginx/pkg/apis/ingress"
)

// getEndpointsFromSlices returns a list of Endpoint structs for a given service/target port combination.
func getEndpointsFromSlices(s *corev1.Service, port *corev1.ServicePort, proto corev1.Protocol, zoneForHints string,
	getServiceEndpointsSlices func(string) ([]*discoveryv1beta1.EndpointSlice, error),
) []ingress.Endpoint {
	upsServers := []ingress.Endpoint{}

	if s == nil || port == nil {
		return upsServers
	}

	// using a map avoids duplicated upstream servers when the service
	// contains multiple port definitions sharing the same targetport
	processedUpstreamServers := make(map[string]struct{})

	svcKey := k8s.MetaNamespaceKey(s)
	//var useTopologyHints bool

	// ExternalName services
	if s.Spec.Type == corev1.ServiceTypeExternalName {
		if ip := net.ParseIP(s.Spec.ExternalName); s.Spec.ExternalName == "localhost" ||
			(ip != nil && ip.IsLoopback()) {
			klog.Errorf("Invalid attempt to use localhost name %s in %q", s.Spec.ExternalName, svcKey)
			return upsServers
		}

		klog.V(3).Infof("Ingress using Service %q of type ExternalName.", svcKey)
		targetPort := port.TargetPort.IntValue()
		// if the externalName is not an IP address we need to validate is a valid FQDN
		if net.ParseIP(s.Spec.ExternalName) == nil {
			externalName := strings.TrimSuffix(s.Spec.ExternalName, ".")
			if errs := validation.IsDNS1123Subdomain(externalName); len(errs) > 0 {
				klog.Errorf("Invalid DNS name %s: %v", s.Spec.ExternalName, errs)
				return upsServers
			}
		}

		return append(upsServers, ingress.Endpoint{
			Address: s.Spec.ExternalName,
			Port:    fmt.Sprintf("%v", targetPort),
		})
	}

	klog.V(3).Infof("Getting Endpoints from endpointSlices for Service %q and port %v", svcKey, port.String())
	epss, err := getServiceEndpointsSlices(svcKey)
	if err != nil {
		klog.Warningf("Error obtaining Endpoints for Service %q: %v", svcKey, err)
		return upsServers
	}
	// loop over all endpointSlices generated for service
	for _, eps := range epss {
		var ports []int32
		if len(eps.Ports) == 0 && port.TargetPort.Type == intstr.Int {
			// When ports is empty, it indicates that there are no defined ports, using svc targePort if it's a number
			klog.V(3).Infof("No ports found on endpointSlice, using service TargetPort %v for Service %q", port.String(), svcKey)
			ports = append(ports, port.TargetPort.IntVal)
		} else {
			for _, epPort := range eps.Ports {
				if !reflect.DeepEqual(*epPort.Protocol, proto) {
                                        klog.V(3).Infof("Skipping port %v due to protocol mismatch", epPort.Name)
					continue
				}
				var targetPort int32
				if port.Name == "" {
					// port.Name is optional if there is only one port
					targetPort = *epPort.Port
				} else if port.Name == *epPort.Name {
					targetPort = *epPort.Port
				}
				if targetPort == 0 && port.TargetPort.Type == intstr.Int {
					// use service target port if it's a number and no port name matched
					// https://github.com/kubernetes/ingress-nginx/issues/7390
					targetPort = port.TargetPort.IntVal
				}
				if targetPort == 0 {
					continue
				}
				ports = append(ports, targetPort)
			}
		}
                  // 修复 epPort 作用域问题
                 for _, targetPort := range ports {
                        for _, ep := range eps.Endpoints {
                                if (ep.Conditions.Ready != nil) && *ep.Conditions.Ready {
                                        for _, epAddress := range ep.Addresses {
                                                hostPort := net.JoinHostPort(epAddress, strconv.Itoa(int(targetPort)))
                                                if _, exists := processedUpstreamServers[hostPort]; !exists {
                                                        upsServers = append(upsServers, ingress.Endpoint{
                                                                Address: epAddress,
                                                                Port:    fmt.Sprintf("%v", targetPort),
                                                                Target:  ep.TargetRef,
                                                        })
                                                        processedUpstreamServers[hostPort] = struct{}{}
                                                }
				}
				}
			}
		}
   }

	klog.V(3).Infof("Endpoints found for Service %q: %v", svcKey, upsServers)
	return upsServers
}
