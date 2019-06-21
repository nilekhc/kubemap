package kubemap

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	apps_v1beta2 "k8s.io/api/apps/v1beta2"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

func kubemapper(obj interface{}, store cache.Store) ([]MapResult, error) {
	object := obj.(ResourceEvent)

	mappedResource, mapErr := resourceMapper(object, store)
	if mapErr != nil {
		return []MapResult{}, mapErr
	}

	storeErr := updateStore(mappedResource, store)
	if storeErr != nil {
		return []MapResult{}, storeErr
	}

	return mappedResource, nil
}

func resourceMapper(obj ResourceEvent, store cache.Store) ([]MapResult, error) {
	switch obj.ResourceType {
	case "ingress":
		// mappedIngress, err := mapIngressObj(obj, store)
		// if err != nil {
		// 	return []MapResult{}, err
		// }

		// return mappedIngress, nil
	case "service":
		mappedService, err := mapServiceObj(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return []MapResult{
			mappedService,
		}, nil
	case "deployment":
		mappedDeployment, err := mapDeploymentObj(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return []MapResult{
			mappedDeployment,
		}, nil
		// case "replicaset":
		// 	mappedReplicaSet, err := mapReplicaSetObj(obj, store)
		// 	if err != nil {
		// 		return []MapResult{}, err
		// 	}

		// 	return []MapResult{
		// 		mappedReplicaSet,
		// 	}, nil
		// case "pod":
		// 	mappedPod, err := mapPodObj(obj, store)
		// 	if err != nil {
		// 		return []MapResult{}, err
		// 	}

		// 	return []MapResult{
		// 		mappedPod,
		// 	}, nil
	}

	return []MapResult{}, fmt.Errorf("Resource type '%s' is not supported for mapping", obj.ResourceType)
}

func mapServiceObj(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var service core_v1.Service
	var namespaceKeys []string

	if obj.Event != nil {
		service = *obj.Event.(*core_v1.Service).DeepCopy()
	}

	keys := store.ListKeys()
	for _, key := range keys {
		if len(strings.Split(key, "/")) > 0 {
			if strings.Split(key, "/")[0] == obj.Namespace {
				namespaceKeys = append(namespaceKeys, key)
			}
		}
	}

	for _, namespaceKey := range namespaceKeys {
		metaIdentifierString := strings.Split(namespaceKey, "/")[1]
		metaIdentifier := MetaIdentifier{}

		json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

		//Try matching with Service
		for _, svcID := range metaIdentifier.ServicesIdentifier {
			if reflect.DeepEqual(service.Spec.Selector, svcID) {
				//Service and deployment matches. Add service to this mapped resource
				mappedResource, _ := getObjectFromStore(namespaceKey, store)

				for i, mappedService := range mappedResource.Kube.Services {
					if mappedService.Name == service.Name {
						mappedResource.Kube.Services[i] = service

						return MapResult{
							Action:         "Updated",
							Key:            namespaceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}
			}
		}

		//Try matching with Deployment
		for _, depID := range metaIdentifier.DeploymentsIdentifier {
			if reflect.DeepEqual(service.Spec.Selector, depID) {
				//Service and deployment matches. Add service to this mapped resource
				mappedResource, _ := getObjectFromStore(namespaceKey, store)

				for i, mappedService := range mappedResource.Kube.Services {
					if mappedService.Name == service.Name {
						mappedResource.Kube.Services[i] = service

						return MapResult{
							Action:         "Updated",
							Key:            namespaceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}

				mappedResource.Kube.Services = append(mappedResource.Kube.Services, service)
				return MapResult{
					Action:         "Updated",
					Key:            namespaceKey,
					IsMapped:       true,
					MappedResource: mappedResource,
				}, nil
			}
		}
	}

	//Didn't find any match. Create new resource
	newMappedService := MappedResource{}
	newMappedService.CommonLabel = service.Name
	newMappedService.CurrentType = "service"
	newMappedService.Namespace = service.Namespace
	newMappedService.Kube.Services = append(newMappedService.Kube.Services, service)

	return MapResult{
		Action:         "Added",
		IsMapped:       true,
		MappedResource: newMappedService,
	}, nil
}

func mapDeploymentObj(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var deployment apps_v1beta2.Deployment
	var namespaceKeys []string

	if obj.Event != nil {
		deployment = *obj.Event.(*apps_v1beta2.Deployment).DeepCopy()
	}

	keys := store.ListKeys()
	for _, key := range keys {
		if len(strings.Split(key, "/")) > 0 {
			if strings.Split(key, "/")[0] == obj.Namespace {
				namespaceKeys = append(namespaceKeys, key)
			}
		}
	}

	for _, namespaceKey := range namespaceKeys {
		metaIdentifierString := strings.Split(namespaceKey, "/")[1]
		metaIdentifier := MetaIdentifier{}

		json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

		//Try matching with Service
		for _, svcID := range metaIdentifier.ServicesIdentifier {
			if reflect.DeepEqual(deployment.Spec.Selector.MatchLabels, svcID) {
				//Service and deployment matches. Add service to this mapped resource
				mappedResource, _ := getObjectFromStore(namespaceKey, store)

				for i, mappedDeployment := range mappedResource.Kube.Deployments {
					if mappedDeployment.Name == deployment.Name {
						mappedResource.Kube.Deployments[i] = deployment

						return MapResult{
							Action:         "Updated",
							Key:            namespaceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}

				mappedResource.Kube.Deployments = append(mappedResource.Kube.Deployments, deployment)
				return MapResult{
					Action:         "Updated",
					Key:            namespaceKey,
					IsMapped:       true,
					MappedResource: mappedResource,
				}, nil
			}
		}

		//Try matching with Deployment
		for _, depID := range metaIdentifier.DeploymentsIdentifier {
			if reflect.DeepEqual(deployment.Spec.Selector.MatchLabels, depID) {
				//Service and deployment matches. Add service to this mapped resource
				mappedResource, _ := getObjectFromStore(namespaceKey, store)

				for i, mappedDeployment := range mappedResource.Kube.Deployments {
					if mappedDeployment.Name == deployment.Name {
						mappedResource.Kube.Deployments[i] = deployment

						return MapResult{
							Action:         "Updated",
							Key:            namespaceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}
			}
		}
	}

	//Didn't find any match. Create new resource
	newMappedService := MappedResource{}
	newMappedService.CommonLabel = deployment.Name
	newMappedService.CurrentType = "deployment"
	newMappedService.Namespace = deployment.Namespace
	newMappedService.Kube.Deployments = append(newMappedService.Kube.Deployments, deployment)

	return MapResult{
		Action:         "Added",
		IsMapped:       true,
		MappedResource: newMappedService,
	}, nil
}
