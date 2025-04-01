/*
Copyright 2022 The Kubernetes Authors.

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

package store

import (
	"fmt"
	"strings"

	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"
	//apiNames "k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/tools/cache"
        //"k8s.io/apimachinery/pkg/api/validation"
)

// EndpointSliceLister makes a Store that lists Endpoints.
type EndpointSliceLister struct {
	cache.Store
}

// MatchByKey returns the EndpointsSlices of the Service matching key in the local Endpoint Store.
func (s *EndpointSliceLister) MatchByKey(key string) ([]*discoveryv1beta1.EndpointSlice, error) {
	var eps []*discoveryv1beta1.EndpointSlice
	keyNsLen := strings.Index(key, "/")
	if keyNsLen < -1 {
		keyNsLen = 0
	} else {
		// count '/' char
		keyNsLen++
	}
	// filter endpointSlices owned by svc
	for _, listKey := range s.ListKeys() {
                if !strings.HasPrefix(listKey, key) {
			continue
		}
		epss, exists, err := s.GetByKey(listKey)
		if exists && err == nil {
			// check for svc owner label
			if svcName, ok := epss.(*discoveryv1beta1.EndpointSlice).ObjectMeta.GetLabels()[discoveryv1beta1.LabelServiceName]; ok {
				namespace := epss.(*discoveryv1beta1.EndpointSlice).ObjectMeta.GetNamespace()
				if key == fmt.Sprintf("%s/%s", namespace, svcName) {
					eps = append(eps, epss.(*discoveryv1beta1.EndpointSlice))
				}
			}
		}
	}
	if len(eps) == 0 {
		return nil, NotExistsError(key)
	}
	return eps, nil
}
