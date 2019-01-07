/*
Copyright 2017 the Heptio Ark contributors.

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

package restore

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	api "github.com/heptio/velero/pkg/apis/velero/v1"
)

const annotationLastAppliedConfig = "kubectl.kubernetes.io/last-applied-configuration"

type serviceAction struct {
	log logrus.FieldLogger
}

func NewServiceAction(logger logrus.FieldLogger) ItemAction {
	return &serviceAction{log: logger}
}

func (a *serviceAction) AppliesTo() (ResourceSelector, error) {
	return ResourceSelector{
		IncludedResources: []string{"services"},
	}, nil
}

func (a *serviceAction) Execute(obj runtime.Unstructured, restore *api.Restore) (runtime.Unstructured, error, error) {
	service := new(corev1api.Service)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), service); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	if service.Spec.ClusterIP != "None" {
		service.Spec.ClusterIP = ""
	}

	if err := deleteNodePorts(service); err != nil {
		return nil, nil, err
	}

	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(service)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	return &unstructured.Unstructured{Object: res}, nil, nil
}

func getPreservedPorts(obj runtime.Unstructured) (map[string]bool, error) {
	preservedPorts := map[string]bool{}
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if lac, ok := metadata.GetAnnotations()[annotationLastAppliedConfig]; ok {
		var svc corev1api.Service
		if err := json.Unmarshal([]byte(lac), &svc); err != nil {
			return nil, errors.WithStack(err)
		}
		for _, port := range svc.Spec.Ports {
			if port.NodePort > 0 {
				preservedPorts[port.Name] = true
			}
		}
	}
	return preservedPorts, nil
}

func deleteNodePorts(service *corev1api.Service) error {
	if service.Spec.Type == corev1api.ServiceTypeExternalName {
		return nil
	}

	// find any NodePorts whose values were explicitly specified according
	// to the last-applied-config annotation. We'll retain these values, and
	// clear out any other (presumably auto-assigned) NodePort values.
	explicitNodePorts := sets.NewString()
	lastAppliedConfig, ok := service.Annotations[annotationLastAppliedConfig]
	if ok {
		appliedService := new(corev1api.Service)
		if err := json.Unmarshal([]byte(lastAppliedConfig), appliedService); err != nil {
			return errors.WithStack(err)
		}

		for _, port := range appliedService.Spec.Ports {
			if port.NodePort > 0 {
				explicitNodePorts.Insert(port.Name)
			}
		}
	}

	for i, port := range service.Spec.Ports {
		if !explicitNodePorts.Has(port.Name) {
			service.Spec.Ports[i].NodePort = 0
		}
	}

	return nil

	// preservedPorts, err := getPreservedPorts(obj)
	// if err != nil {
	// 	return err
	// }

	// res, found, err := unstructured.NestedFieldNoCopy(obj.UnstructuredContent(), "spec", "ports")
	// if err != nil {
	// 	return errors.WithStack(err)
	// }
	// if !found {
	// 	return errors.New(".spec.ports not found")
	// }

	// ports, ok := res.([]interface{})
	// if !ok {
	// 	return errors.Errorf("unexpected type for .spec.ports %T", res)
	// }

	// for _, port := range ports {
	// 	p := port.(map[string]interface{})
	// 	var name string
	// 	if nameVal, ok := p["name"]; ok {
	// 		name = nameVal.(string)
	// 	}
	// 	if preservedPorts[name] {
	// 		continue
	// 	}
	// 	delete(p, "nodePort")
	// }
	// return nil
}
