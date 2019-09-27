package kubemap

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	apps_v1beta2 "k8s.io/api/apps/v1beta2"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

func (m *Mapper) kubemapper(obj interface{}, store cache.Store) ([]MapResult, error) {
	object := obj.(ResourceEvent)
	m.debug(fmt.Sprintf("\nProcessing object - \n K8s Type - %s\n Name - %s\n Namespace - %s\n", object.ResourceType, object.Name, object.Namespace))

	mappedResource, mapErr := m.resourceMapper(object, store)
	if mapErr != nil {
		return []MapResult{}, mapErr
	}

	storeErr := m.updateStore(mappedResource, store)
	if storeErr != nil {
		m.warn(fmt.Sprintf("\nError while updating store - %v\n", storeErr))
		return []MapResult{}, storeErr
	}

	return mappedResource, nil
}

func (m *Mapper) resourceMapper(obj ResourceEvent, store cache.Store) ([]MapResult, error) {
	switch obj.ResourceType {
	case "ingress":
		mappedIngress, err := m.mapIngressObj(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return mappedIngress, nil
	case "service":
		mappedService, err := m.mapServiceObj(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return []MapResult{
			mappedService,
		}, nil
	case "deployment":
		mappedDeployment, err := m.mapDeploymentObj(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return []MapResult{
			mappedDeployment,
		}, nil
	case "replicaset":
		mappedReplicaSet, err := m.mapReplicaSetObj(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return []MapResult{
			mappedReplicaSet,
		}, nil
	case "pod":
		mappedPod, err := m.mapPodObj(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return []MapResult{
			mappedPod,
		}, nil
	}

	return []MapResult{}, fmt.Errorf("Resource type '%s' is not supported for mapping", obj.ResourceType)
}

func (m *Mapper) mapIngressObj(obj ResourceEvent, store cache.Store) ([]MapResult, error) {
	var ingress ext_v1beta1.Ingress
	var ingressBackendServices []string

	if obj.Event != nil {
		ingress = *obj.Event.(*ext_v1beta1.Ingress).DeepCopy()
		//Get all services from ingress rules
		for _, ingressRule := range ingress.Spec.Rules {
			if ingressRule.IngressRuleValue.HTTP != nil {
				for _, ingressRuleValueHTTPPath := range ingressRule.IngressRuleValue.HTTP.Paths {
					if ingressRuleValueHTTPPath.Backend.ServiceName != "" {
						ingressBackendServices = append(ingressBackendServices, ingressRuleValueHTTPPath.Backend.ServiceName)
					}
				}
			}
		}

		ingressBackendServices = removeDuplicateStrings(ingressBackendServices)

		if obj.EventType == "ADDED" {
			return m.addIngress(store, obj, ingress, ingressBackendServices)
		} else if obj.EventType == "UPDATED" {
			mapResults := []MapResult{}

			deleteResults, delErr := m.deleteIngress(store, obj)
			if delErr != nil {
				return []MapResult{}, delErr
			}

			addResults, addErr := m.addIngress(store, obj, ingress, ingressBackendServices)
			if addErr != nil {
				return []MapResult{}, addErr
			}

			mapResults = append(mapResults, addResults...)
			mapResults = append(mapResults, deleteResults...)

			return mapResults, nil
		}
	}

	//Handle Delete
	if obj.EventType == "DELETED" {
		return m.deleteIngress(store, obj)
	}
	return []MapResult{}, nil
}

func (m *Mapper) addIngress(store cache.Store, obj ResourceEvent, ingress ext_v1beta1.Ingress, ingressBackendServices []string) ([]MapResult, error) {
	var mapResults []MapResult
	var namespaceKeys []string

	keys := store.ListKeys()
	for _, b64Key := range keys {
		encodedKey, _ := base64.StdEncoding.DecodeString(b64Key)
		key := fmt.Sprintf("%s", encodedKey)
		if len(strings.Split(key, "$")) > 0 {
			if strings.Split(key, "$")[0] == obj.Namespace {
				namespaceKeys = append(namespaceKeys, key)
			}
		}
	}

	isMatched := false
	for _, namespaceKey := range namespaceKeys {
		metaIdentifierString := strings.Split(namespaceKey, "$")[1]
		metaIdentifier := MetaIdentifier{}

		json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

		for _, ingressBackendService := range ingressBackendServices {
			//Try matching with Service
			for _, serviceName := range metaIdentifier.ServicesIdentifier.Names {
				if serviceName == ingressBackendService {
					//Get object

					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					isUpdated := false
					for i, mappedIngress := range mappedResource.Kube.Ingresses {
						if mappedIngress.Name == ingress.Name {
							mappedResource.Kube.Ingresses[i] = ingress
							isUpdated = true

							mapResults = append(mapResults,
								MapResult{
									Action:         "Updated",
									Key:            namespaceKey,
									IsMapped:       true,
									MappedResource: mappedResource,
									Message:        fmt.Sprintf("Ingress %s updated in Common Label %s", ingress.Name, mappedResource.CommonLabel),
								},
							)
						}
					}

					if !isUpdated {
						mappedResource.Kube.Ingresses = append(mappedResource.Kube.Ingresses, ingress)

						mapResults = append(mapResults,
							MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
								Message:        fmt.Sprintf("Ingress %s added in Common Label %s", ingress.Name, mappedResource.CommonLabel),
							},
						)
					}
					isMatched = true
				}
			}
		}
	}
	if !isMatched {
		//Create new object with ingress
		newMappedService := MappedResource{}
		newMappedService.CommonLabel = ingress.Name
		newMappedService.CurrentType = "service"
		newMappedService.Namespace = ingress.Namespace
		newMappedService.Kube.Ingresses = append(newMappedService.Kube.Ingresses, ingress)

		mapResults = append(mapResults,
			MapResult{
				Action:         "Added",
				IsMapped:       true,
				MappedResource: newMappedService,
				Message:        fmt.Sprintf("New ingress %s created with Common Label %s", ingress.Name, newMappedService.CommonLabel),
			},
		)
	}

	//Update store right sway. Helps in B/G scenarios of ingress
	m.updateStore(mapResults, store)

	//Set IsStoreUpdated to true
	for i := range mapResults {
		mapResults[i].IsStoreUpdated = true
	}

	return mapResults, nil
}

func (m *Mapper) deleteIngress(store cache.Store, obj ResourceEvent) ([]MapResult, error) {
	m.info(fmt.Sprintf("\nDELETE received - \n K8s Type - %s\n Name - %s\n Namespace - %s\n", obj.ResourceType, obj.Name, obj.Namespace))

	var ingressBackendServices, namespaceKeys []string
	var mapResults []MapResult

	keys := store.ListKeys()
	for _, b64Key := range keys {
		encodedKey, _ := base64.StdEncoding.DecodeString(b64Key)
		key := fmt.Sprintf("%s", encodedKey)
		if len(strings.Split(key, "$")) > 0 {
			if strings.Split(key, "$")[0] == obj.Namespace {
				namespaceKeys = append(namespaceKeys, key)
			}
		}
	}

	for _, namespaceKey := range namespaceKeys {
		metaIdentifierString := strings.Split(namespaceKey, "$")[1]
		metaIdentifier := MetaIdentifier{}

		json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

		for _, ingressName := range metaIdentifier.IngressIdentifier.Names {
			if ingressName == obj.Name {
				//This contains possible list of services to which this ingress is attached.
				//Delete ingress from it.
				ingressBackendServices = metaIdentifier.IngressIdentifier.IngressBackendServices
			}
		}
	}

	for _, ingressBackendService := range ingressBackendServices {
		for _, namespaceKey := range namespaceKeys {
			metaIdentifierString := strings.Split(namespaceKey, "$")[1]
			metaIdentifier := MetaIdentifier{}

			json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

			var newIngressSet []ext_v1beta1.Ingress
			for _, serviceName := range metaIdentifier.ServicesIdentifier.Names {
				if serviceName == ingressBackendService {
					//Services matched. See if ingress is present. If it is, then delete it.
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					newIngressSet = nil
					isPresent := false
					for _, mappedIngress := range mappedResource.Kube.Ingresses {
						if mappedIngress.Name == obj.Name {
							isPresent = true
						} else {
							newIngressSet = append(newIngressSet, mappedIngress)
						}
					}

					if isPresent {
						if len(mappedResource.Kube.Services) > 0 || len(mappedResource.Kube.Deployments) > 0 || len(mappedResource.Kube.ReplicaSets) > 0 || len(mappedResource.Kube.Pods) > 0 || len(mappedResource.Kube.Ingresses) > 1 {
							//It has another resources.
							mappedResource.Kube.Ingresses = nil
							mappedResource.Kube.Ingresses = newIngressSet

							mapResults = append(mapResults,
								MapResult{
									Action:         "Updated",
									Key:            namespaceKey,
									IsMapped:       true,
									MappedResource: mappedResource,
									Message:        fmt.Sprintf("Ingress %s deleted from Common Label %s", obj.Name, mappedResource.CommonLabel),
								},
							)
						} else {
							mapResults = append(mapResults,
								MapResult{
									Action:         "Deleted",
									Key:            namespaceKey,
									IsMapped:       true,
									CommonLabel:    mappedResource.CommonLabel,
									MappedResource: mappedResource,
									Message:        fmt.Sprintf("Ingress %s deleted from Common Label %s", obj.Name, mappedResource.CommonLabel),
								},
							)
						}
					}
				}
			}
		}
	}

	//Update store right sway. Helps in B/G scenarios of ingress
	m.updateStore(mapResults, store)

	//Set IsStoreUpdated to true
	for i := range mapResults {
		mapResults[i].IsStoreUpdated = true
	}

	m.info(fmt.Sprintf("\nDELETE Completed - \n K8s Type - %s\n Name - %s\n Namespace - %s\n", obj.ResourceType, obj.Name, obj.Namespace))
	return mapResults, nil
}

func (m *Mapper) ingressCheck(mappedResource MappedResource, serviceName string, namespaceKeys []string, store cache.Store) (MappedResource, []string) {
	var oldIngressDeleteKeys []string
	for _, namespaceKey := range namespaceKeys {
		metaIdentifierString := strings.Split(namespaceKey, "$")[1]
		metaIdentifier := MetaIdentifier{}

		json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)
		if metaIdentifier.DeploymentsIdentifier.MatchLabels == nil && metaIdentifier.PodsIdentifier == nil && metaIdentifier.ReplicaSetsIdentifier == nil && metaIdentifier.ServicesIdentifier.MatchLabels == nil && metaIdentifier.IngressIdentifier.IngressBackendServices != nil {
			//Its an object with just ingress
			for _, ingressBackendService := range metaIdentifier.IngressIdentifier.IngressBackendServices {
				if ingressBackendService == serviceName {
					//This ingress belongs to this service. Add it
					// ingressMappedResource, _ := getObjectFromStore(namespaceKey, store)
					ingressMappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)
					for _, loneIngress := range ingressMappedResource.Kube.Ingresses {
						mappedResource.Kube.Ingresses = append(mappedResource.Kube.Ingresses, loneIngress)
					}
					oldIngressDeleteKeys = append(oldIngressDeleteKeys, namespaceKey)
				}
			}
		}

		//Check for non lone ingress which might part of more than one services and already mapped to one of them.
		//for _, ingressBackendService := range metaIdentifier.IngressIdentifier.IngressBackendServices {
		//if ingressBackendService == serviceName {
		//This ingress belongs to this service. Add it

		// ingressMappedResource, _ := getObjectFromStore(namespaceKey, store)
		ingressMappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

		for _, mappedIngress := range mappedResource.Kube.Ingresses {
			for _, mappedIngressResource := range ingressMappedResource.Kube.Ingresses {
				if mappedIngress.Name != mappedIngressResource.Name {
					var currentIngressBackendServices []string
					for _, ingressRule := range mappedIngressResource.Spec.Rules {
						if ingressRule.IngressRuleValue.HTTP != nil {
							for _, ingressRuleValueHTTPPath := range ingressRule.IngressRuleValue.HTTP.Paths {
								if ingressRuleValueHTTPPath.Backend.ServiceName != "" {
									currentIngressBackendServices = append(currentIngressBackendServices, ingressRuleValueHTTPPath.Backend.ServiceName)
								}
							}
						}
					}

					for _, currentIngressBackendService := range currentIngressBackendServices {
						if currentIngressBackendService == serviceName {
							mappedResource.Kube.Ingresses = append(mappedResource.Kube.Ingresses, mappedIngressResource)
						}
					}
				}
			}
		}
		//}
		//}
	}

	return mappedResource, oldIngressDeleteKeys
}

func (m *Mapper) mapServiceObj(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var service core_v1.Service
	var namespaceKeys []string

	if obj.Event != nil {
		service = *obj.Event.(*core_v1.Service).DeepCopy()

		keys := store.ListKeys()
		for _, b64Key := range keys {
			encodedKey, _ := base64.StdEncoding.DecodeString(b64Key)
			key := fmt.Sprintf("%s", encodedKey)
			if len(strings.Split(key, "$")) > 0 {
				if strings.Split(key, "$")[0] == obj.Namespace {
					namespaceKeys = append(namespaceKeys, key)
				}
			}
		}

		for _, namespaceKey := range namespaceKeys {
			metaIdentifierString := strings.Split(namespaceKey, "$")[1]
			metaIdentifier := MetaIdentifier{}

			json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

			//Try matching with Service
			for _, svcID := range metaIdentifier.ServicesIdentifier.MatchLabels {
				if reflect.DeepEqual(service.Spec.Selector, svcID) {
					//Service and deployment matches. Add service to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedService := range mappedResource.Kube.Services {
						if mappedService.Name == service.Name {
							mappedResource.Kube.Services[i] = service

							newMappedResource, deleteKeys := m.ingressCheck(mappedResource, service.Name, namespaceKeys, store)
							deleteKeys = append(deleteKeys, namespaceKey)
							deleteKeys = removeDuplicateStrings(deleteKeys)

							return MapResult{
								Action:         "Updated",
								DeleteKeys:     deleteKeys,
								IsMapped:       true,
								MappedResource: newMappedResource,
								Message:        fmt.Sprintf("Service %s updated in Common Label %s after matching with service.", service.Name, mappedResource.CommonLabel),
							}, nil
						}
					}
				}
			}

			//Try matching with Deployment
			for _, depID := range metaIdentifier.DeploymentsIdentifier.MatchLabels {
				if reflect.DeepEqual(service.Spec.Selector, depID) {
					//Service and deployment matches. Add service to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedService := range mappedResource.Kube.Services {
						if mappedService.Name == service.Name {
							mappedResource.Kube.Services[i] = service

							newMappedResource, deleteKeys := m.ingressCheck(mappedResource, service.Name, namespaceKeys, store)
							deleteKeys = append(deleteKeys, namespaceKey)
							deleteKeys = removeDuplicateStrings(deleteKeys)

							return MapResult{
								Action:         "Updated",
								DeleteKeys:     deleteKeys,
								IsMapped:       true,
								MappedResource: newMappedResource,
								Message:        fmt.Sprintf("Service %s updated in Common Label %s after matching with deployment.", service.Name, mappedResource.CommonLabel),
							}, nil
						}
					}

					mappedResource.Kube.Services = append(mappedResource.Kube.Services, service)
					if len(mappedResource.Kube.Services) < 2 { //Set Common Label to service name.
						mappedResource.CommonLabel = service.Name
					}

					newMappedResource, deleteKeys := m.ingressCheck(mappedResource, service.Name, namespaceKeys, store)
					deleteKeys = append(deleteKeys, namespaceKey)
					deleteKeys = removeDuplicateStrings(deleteKeys)

					return MapResult{
						Action:         "Updated",
						DeleteKeys:     deleteKeys,
						IsMapped:       true,
						MappedResource: newMappedResource,
						Message:        fmt.Sprintf("Service %s is added to Common Label %s after matching with deployment.", service.Name, mappedResource.CommonLabel),
					}, nil
				}
			}

			//Try matching with Replica set
			for _, rsID := range metaIdentifier.ReplicaSetsIdentifier {
				serviceMatchedLabels := make(map[string]string)
				for rsKey, rsValue := range rsID.MatchLabels {
					if val, ok := service.Spec.Selector[rsKey]; ok {
						if val == rsValue {
							serviceMatchedLabels[rsKey] = rsValue
						}
					}
				}
				if reflect.DeepEqual(service.Spec.Selector, serviceMatchedLabels) {
					//Service and deployment matches. Add service to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedService := range mappedResource.Kube.Services {
						if mappedService.Name == service.Name {
							mappedResource.Kube.Services[i] = service

							newMappedResource, deleteKeys := m.ingressCheck(mappedResource, service.Name, namespaceKeys, store)
							deleteKeys = append(deleteKeys, namespaceKey)
							deleteKeys = removeDuplicateStrings(deleteKeys)

							return MapResult{
								Action:         "Updated",
								DeleteKeys:     deleteKeys,
								IsMapped:       true,
								MappedResource: newMappedResource,
								Message:        fmt.Sprintf("Service %s is updated in Common Label %s after matching with replica set.", service.Name, mappedResource.CommonLabel),
							}, nil
						}
					}

					mappedResource.Kube.Services = append(mappedResource.Kube.Services, service)
					if len(mappedResource.Kube.Services) < 2 { //Set Common Label to service name.
						mappedResource.CommonLabel = service.Name
					}
					newMappedResource, deleteKeys := m.ingressCheck(mappedResource, service.Name, namespaceKeys, store)
					deleteKeys = append(deleteKeys, namespaceKey)
					deleteKeys = removeDuplicateStrings(deleteKeys)

					return MapResult{
						Action:         "Updated",
						DeleteKeys:     deleteKeys,
						IsMapped:       true,
						MappedResource: newMappedResource,
						Message:        fmt.Sprintf("Service %s is added to Common Label %s after matching with replica set.", service.Name, mappedResource.CommonLabel),
					}, nil
				}
			}

			//Try matching with Pods
			for _, podID := range metaIdentifier.PodsIdentifier {
				serviceMatchedLabels := make(map[string]string)
				for podKey, podValue := range podID.MatchLabels {
					if val, ok := service.Spec.Selector[podKey]; ok {
						if val == podValue {
							serviceMatchedLabels[podKey] = podValue
						}
					}
				}
				if reflect.DeepEqual(service.Spec.Selector, serviceMatchedLabels) {
					//Service and deployment matches. Add service to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedService := range mappedResource.Kube.Services {
						if mappedService.Name == service.Name {
							mappedResource.Kube.Services[i] = service

							newMappedResource, deleteKeys := m.ingressCheck(mappedResource, service.Name, namespaceKeys, store)
							deleteKeys = append(deleteKeys, namespaceKey)
							deleteKeys = removeDuplicateStrings(deleteKeys)

							return MapResult{
								Action:         "Updated",
								DeleteKeys:     deleteKeys,
								IsMapped:       true,
								MappedResource: newMappedResource,
								Message:        fmt.Sprintf("Service %s is updated in Common Label %s after matching with pod.", service.Name, mappedResource.CommonLabel),
							}, nil
						}
					}

					mappedResource.Kube.Services = append(mappedResource.Kube.Services, service)
					if len(mappedResource.Kube.Services) < 2 { //Set Common Label to service name.
						mappedResource.CommonLabel = service.Name
					}
					newMappedResource, deleteKeys := m.ingressCheck(mappedResource, service.Name, namespaceKeys, store)
					deleteKeys = append(deleteKeys, namespaceKey)
					deleteKeys = removeDuplicateStrings(deleteKeys)

					return MapResult{
						Action: "Updated",
						// Key:            namespaceKey,
						DeleteKeys:     deleteKeys,
						IsMapped:       true,
						MappedResource: newMappedResource,
						Message:        fmt.Sprintf("Service %s is added to Common Label %s after matching with pod.", service.Name, mappedResource.CommonLabel),
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

		newMappedResourceWithIngress, deleteKeys := m.ingressCheck(newMappedService, service.Name, namespaceKeys, store)
		deleteKeys = removeDuplicateStrings(deleteKeys)

		return MapResult{
			Action:         "Added",
			IsMapped:       true,
			DeleteKeys:     deleteKeys,
			MappedResource: newMappedResourceWithIngress,
			Message:        fmt.Sprintf("New service %s is added with Common Label %s", service.Name, newMappedResourceWithIngress.CommonLabel),
		}, nil
	}

	//Handle Delete
	if obj.EventType == "DELETED" {
		m.info(fmt.Sprintf("\nDELETE received - \n K8s Type - %s\n Name - %s\n Namespace - %s\n", obj.ResourceType, obj.Name, obj.Namespace))

		keys := store.ListKeys()
		for _, b64Key := range keys {
			encodedKey, _ := base64.StdEncoding.DecodeString(b64Key)
			key := fmt.Sprintf("%s", encodedKey)
			if len(strings.Split(key, "$")) > 0 {
				if strings.Split(key, "$")[0] == obj.Namespace {
					namespaceKeys = append(namespaceKeys, key)
				}
			}
		}

		var newSvcSet []core_v1.Service
		for _, namespaceKey := range namespaceKeys {
			metaIdentifierString := strings.Split(namespaceKey, "$")[1]
			metaIdentifier := MetaIdentifier{}

			json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

			for _, mappedSvcName := range metaIdentifier.ServicesIdentifier.Names {
				if mappedSvcName == obj.Name {
					//Pod is being deleted.
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					newSvcSet = nil
					for _, mappedService := range mappedResource.Kube.Services {
						if mappedService.Name != obj.Name {
							newSvcSet = append(newSvcSet, mappedService)
						}
					}

					if len(mappedResource.Kube.Ingresses) > 0 || len(mappedResource.Kube.Deployments) > 0 || len(mappedResource.Kube.ReplicaSets) > 0 || len(mappedResource.Kube.Pods) > 0 || len(mappedResource.Kube.Services) > 1 {
						//It has another resources.
						mappedResource.Kube.Services = nil
						mappedResource.Kube.Services = newSvcSet

						m.info(fmt.Sprintf("\nDELETE Completed - \n K8s Type - %s\n Name - %s\n Namespace - %s\nCL updated.\n", obj.ResourceType, obj.Name, obj.Namespace))
						return MapResult{
							Action:         "Updated",
							Key:            namespaceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
							Message:        fmt.Sprintf("Service %s is deleted from Common Label %s", service.Name, mappedResource.CommonLabel),
						}, nil
					}

					m.info(fmt.Sprintf("\nDELETE Completed. - \n K8s Type - %s\n Name - %s\n Namespace - %s\nCL deleted.\n", obj.ResourceType, obj.Name, obj.Namespace))
					return MapResult{
						Action:         "Deleted",
						Key:            namespaceKey,
						IsMapped:       true,
						CommonLabel:    mappedResource.CommonLabel,
						MappedResource: mappedResource,
						Message:        fmt.Sprintf("Service %s is deleted from Common Label %s", service.Name, mappedResource.CommonLabel),
					}, nil

				}
			}
		}
	}
	return MapResult{}, nil
}

func (m *Mapper) mapDeploymentObj(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var deployment apps_v1beta2.Deployment
	var namespaceKeys []string

	if obj.Event != nil {
		deployment = *obj.Event.(*apps_v1beta2.Deployment).DeepCopy()

		keys := store.ListKeys()
		for _, b64Key := range keys {
			encodedKey, _ := base64.StdEncoding.DecodeString(b64Key)
			key := fmt.Sprintf("%s", encodedKey)
			if len(strings.Split(key, "$")) > 0 {
				if strings.Split(key, "$")[0] == obj.Namespace {
					namespaceKeys = append(namespaceKeys, key)
				}
			}
		}

		for _, namespaceKey := range namespaceKeys {
			metaIdentifierString := strings.Split(namespaceKey, "$")[1]
			metaIdentifier := MetaIdentifier{}

			json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

			//Try matching with Service
			for _, svcID := range metaIdentifier.ServicesIdentifier.MatchLabels {
				if reflect.DeepEqual(deployment.Spec.Selector.MatchLabels, svcID) {
					//Service and deployment matches. Add service to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedDeployment := range mappedResource.Kube.Deployments {
						if mappedDeployment.Name == deployment.Name {
							mappedResource.Kube.Deployments[i] = deployment

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
								Message:        fmt.Sprintf("Deployment %s is updated in Common Label %s after matching with service", deployment.Name, mappedResource.CommonLabel),
							}, nil
						}
					}

					mappedResource.Kube.Deployments = append(mappedResource.Kube.Deployments, deployment)
					return MapResult{
						Action:         "Updated",
						Key:            namespaceKey,
						IsMapped:       true,
						MappedResource: mappedResource,
						Message:        fmt.Sprintf("Deployment %s is added to Common Label %s after matching with service", deployment.Name, mappedResource.CommonLabel),
					}, nil
				}
			}

			//Try matching with Deployment
			for _, depID := range metaIdentifier.DeploymentsIdentifier.MatchLabels {
				if reflect.DeepEqual(deployment.Spec.Selector.MatchLabels, depID) {
					//Service and deployment matches. Add service to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedDeployment := range mappedResource.Kube.Deployments {
						if mappedDeployment.Name == deployment.Name {
							mappedResource.Kube.Deployments[i] = deployment

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
								Message:        fmt.Sprintf("Deployment %s is updated io Common Label %s after matching with deployment", deployment.Name, mappedResource.CommonLabel),
							}, nil
						}
					}
				}
			}

			//Try matching with Replica set
			for _, rsID := range metaIdentifier.ReplicaSetsIdentifier {
				for _, ownerReference := range rsID.OwnerReferences {
					if ownerReference == deployment.Name {
						//Deployment and RS matches. Add deployment to this mapped resource
						// mappedResource, _ := getObjectFromStore(namespaceKey, store)
						mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

						for i, mappedDeployment := range mappedResource.Kube.Deployments {
							if mappedDeployment.Name == deployment.Name {
								mappedResource.Kube.Deployments[i] = deployment

								return MapResult{
									Action:         "Updated",
									Key:            namespaceKey,
									IsMapped:       true,
									MappedResource: mappedResource,
									Message:        fmt.Sprintf("Deployment %s is updated io Common Label %s after matching with replica set", deployment.Name, mappedResource.CommonLabel),
								}, nil
							}
						}

						mappedResource.Kube.Deployments = append(mappedResource.Kube.Deployments, deployment)
						if len(mappedResource.Kube.Deployments) < 2 { //Set Common Label to deployment name.
							mappedResource.CommonLabel = deployment.Name
						}
						return MapResult{
							Action:         "Updated",
							Key:            namespaceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
							Message:        fmt.Sprintf("Deployment %s is added to Common Label %s after matching with replica set", deployment.Name, mappedResource.CommonLabel),
						}, nil
					}
				}
			}

			//Try matching with Pod
			for _, podID := range metaIdentifier.PodsIdentifier {
				podMatchedLabels := make(map[string]string)
				for podKey, podValue := range podID.MatchLabels {
					if val, ok := deployment.Spec.Selector.MatchLabels[podKey]; ok {
						if val == podValue {
							podMatchedLabels[podKey] = podValue
						}
					}
				}

				if reflect.DeepEqual(deployment.Spec.Selector.MatchLabels, podMatchedLabels) {
					//Deployment and RS matches. Add deployment to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedDeployment := range mappedResource.Kube.Deployments {
						if mappedDeployment.Name == deployment.Name {
							mappedResource.Kube.Deployments[i] = deployment

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
								Message:        fmt.Sprintf("Deployment %s is updated io Common Label %s after matching with pod", deployment.Name, mappedResource.CommonLabel),
							}, nil
						}
					}

					mappedResource.Kube.Deployments = append(mappedResource.Kube.Deployments, deployment)
					if len(mappedResource.Kube.Deployments) < 2 { //Set Common Label to deployment name.
						mappedResource.CommonLabel = deployment.Name
					}
					return MapResult{
						Action:         "Updated",
						Key:            namespaceKey,
						IsMapped:       true,
						MappedResource: mappedResource,
						Message:        fmt.Sprintf("Deployment %s is added to Common Label %s after matching with pod", deployment.Name, mappedResource.CommonLabel),
					}, nil
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
			Message:        fmt.Sprintf("New deployment %s is created with Common Label %s", deployment.Name, newMappedService.CommonLabel),
		}, nil
	}

	//Handle Delete
	if obj.EventType == "DELETED" {
		m.info(fmt.Sprintf("\nDELETE received. - \n K8s Type - %s\n Name - %s\n Namespace - %s\n", obj.ResourceType, obj.Name, obj.Namespace))

		keys := store.ListKeys()
		for _, b64Key := range keys {
			encodedKey, _ := base64.StdEncoding.DecodeString(b64Key)
			key := fmt.Sprintf("%s", encodedKey)
			if len(strings.Split(key, "$")) > 0 {
				if strings.Split(key, "$")[0] == obj.Namespace {
					namespaceKeys = append(namespaceKeys, key)
				}
			}
		}

		var newDepSet []apps_v1beta2.Deployment
		for _, namespaceKey := range namespaceKeys {
			metaIdentifierString := strings.Split(namespaceKey, "$")[1]
			metaIdentifier := MetaIdentifier{}

			json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

			for _, mappedDepName := range metaIdentifier.DeploymentsIdentifier.Names {
				if mappedDepName == obj.Name {
					//Pod is being deleted.
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					newDepSet = nil
					for _, mappedDeployment := range mappedResource.Kube.Deployments {
						if mappedDeployment.Name != obj.Name {
							newDepSet = append(newDepSet, mappedDeployment)
						}
					}

					if len(mappedResource.Kube.Ingresses) > 0 || len(mappedResource.Kube.Services) > 0 || len(mappedResource.Kube.ReplicaSets) > 0 || len(mappedResource.Kube.Pods) > 0 || len(mappedResource.Kube.Deployments) > 1 {
						//It has another resources.
						mappedResource.Kube.Deployments = nil
						mappedResource.Kube.Deployments = newDepSet

						m.info(fmt.Sprintf("\nDELETE Completed. - \n K8s Type - %s\n Name - %s\n Namespace - %s\nCL updated.\n", obj.ResourceType, obj.Name, obj.Namespace))
						return MapResult{
							Action:         "Updated",
							Key:            namespaceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
							Message:        fmt.Sprintf("Deployment %s is deleted from Common Label %s", deployment.Name, mappedResource.CommonLabel),
						}, nil
					}

					m.info(fmt.Sprintf("\nDELETE Completed. - \n K8s Type - %s\n Name - %s\n Namespace - %s\nCL deleted.\n", obj.ResourceType, obj.Name, obj.Namespace))
					return MapResult{
						Action:         "Deleted",
						Key:            namespaceKey,
						IsMapped:       true,
						CommonLabel:    mappedResource.CommonLabel,
						MappedResource: mappedResource,
						Message:        fmt.Sprintf("Deployment %s is deleted from Common Label %s", deployment.Name, mappedResource.CommonLabel),
					}, nil

				}
			}
		}
	}
	return MapResult{}, nil
}

func (m *Mapper) mapPodObj(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var pod core_v1.Pod
	var namespaceKeys []string

	if obj.Event != nil {
		pod = *obj.Event.(*core_v1.Pod).DeepCopy()

		keys := store.ListKeys()
		for _, b64Key := range keys {
			encodedKey, _ := base64.StdEncoding.DecodeString(b64Key)
			key := fmt.Sprintf("%s", encodedKey)
			if len(strings.Split(key, "$")) > 0 {
				if strings.Split(key, "$")[0] == obj.Namespace {
					namespaceKeys = append(namespaceKeys, key)
				}
			}
		}

		for _, namespaceKey := range namespaceKeys {
			metaIdentifierString := strings.Split(namespaceKey, "$")[1]
			metaIdentifier := MetaIdentifier{}

			json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

			//Try matching with Service
			for _, svcID := range metaIdentifier.ServicesIdentifier.MatchLabels {
				podMatchedLabels := make(map[string]string)
				for svcKey, svcValue := range svcID {
					if val, ok := pod.Labels[svcKey]; ok {
						if val == svcValue {
							podMatchedLabels[svcKey] = svcValue
						}
					}
				}
				if reflect.DeepEqual(podMatchedLabels, svcID) {
					//Service and pod matches. Add pod to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedPod := range mappedResource.Kube.Pods {
						if mappedPod.Name == pod.Name {
							mappedResource.Kube.Pods[i] = pod

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
								Message:        fmt.Sprintf("Pod %s is updated in Common Label %s after matching with service", pod.Name, mappedResource.CommonLabel),
							}, nil
						}
					}

					mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)
					return MapResult{
						Action:         "Updated",
						Key:            namespaceKey,
						IsMapped:       true,
						MappedResource: mappedResource,
						Message:        fmt.Sprintf("Pod %s is added to Common Label %s after matching with service", pod.Name, mappedResource.CommonLabel),
					}, nil
				}
			}

			//Try matching with Deployment
			for _, depID := range metaIdentifier.DeploymentsIdentifier.MatchLabels {
				podMatchedLabels := make(map[string]string)
				for depKey, depValue := range depID {
					if val, ok := pod.Labels[depKey]; ok {
						if val == depValue {
							podMatchedLabels[depKey] = depValue
						}
					}
				}
				if reflect.DeepEqual(podMatchedLabels, depID) {
					//Service and deployment matches. Add service to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedPod := range mappedResource.Kube.Pods {
						if mappedPod.Name == pod.Name {
							mappedResource.Kube.Pods[i] = pod

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
								Message:        fmt.Sprintf("Pod %s is updated in Common Label %s after matching with deployment", pod.Name, mappedResource.CommonLabel),
							}, nil
						}
					}

					mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)
					return MapResult{
						Action:         "Updated",
						Key:            namespaceKey,
						IsMapped:       true,
						MappedResource: mappedResource,
						Message:        fmt.Sprintf("Pod %s is added to Common Label %s after matching with deployment", pod.Name, mappedResource.CommonLabel),
					}, nil
				}
			}

			//Try matching with RS
			for _, rsID := range metaIdentifier.ReplicaSetsIdentifier {
				podMatchedLabels := make(map[string]string)
				for rsKey, rsValue := range rsID.MatchLabels {
					if val, ok := pod.Labels[rsKey]; ok {
						if val == rsValue {
							podMatchedLabels[rsKey] = rsValue
						}
					}
				}
				if reflect.DeepEqual(podMatchedLabels, rsID.MatchLabels) {
					//Service and deployment matches. Add service to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedPod := range mappedResource.Kube.Pods {
						if mappedPod.Name == pod.Name {
							mappedResource.Kube.Pods[i] = pod

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
								Message:        fmt.Sprintf("Pod %s is updated in Common Label %s after matching with replica set", pod.Name, mappedResource.CommonLabel),
							}, nil
						}
					}

					mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)
					return MapResult{
						Action:         "Updated",
						Key:            namespaceKey,
						IsMapped:       true,
						MappedResource: mappedResource,
						Message:        fmt.Sprintf("Pod %s is added to Common Label %s after matching with replica set", pod.Name, mappedResource.CommonLabel),
					}, nil
				}
			}

			//Try matching with Pod
			for _, podID := range metaIdentifier.PodsIdentifier {
				if reflect.DeepEqual(pod.Labels, podID.MatchLabels) {
					//Service and deployment matches. Add service to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedPod := range mappedResource.Kube.Pods {
						if mappedPod.Name == pod.Name {
							mappedResource.Kube.Pods[i] = pod

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
								Message:        fmt.Sprintf("Pod %s is updated to Common Label %s after matching with pod", pod.Name, mappedResource.CommonLabel),
							}, nil
						}
					}
				}
			}
		}

		//Didn't find any match. Create new resource
		newMappedService := MappedResource{}
		newMappedService.CommonLabel = pod.Name
		newMappedService.CurrentType = "pod"
		newMappedService.Namespace = pod.Namespace
		newMappedService.Kube.Pods = append(newMappedService.Kube.Pods, pod)

		return MapResult{
			Action:         "Added",
			IsMapped:       true,
			MappedResource: newMappedService,
			Message:        fmt.Sprintf("New pod %s is created with Common Label %s", pod.Name, newMappedService.CommonLabel),
		}, nil
	}

	//Handle Delete
	if obj.EventType == "DELETED" {
		m.info(fmt.Sprintf("\nDELETE received. - \n K8s Type - %s\n Name - %s\n Namespace - %s\n", obj.ResourceType, obj.Name, obj.Namespace))

		keys := store.ListKeys()
		for _, b64Key := range keys {
			encodedKey, _ := base64.StdEncoding.DecodeString(b64Key)
			key := fmt.Sprintf("%s", encodedKey)
			if len(strings.Split(key, "$")) > 0 {
				if strings.Split(key, "$")[0] == obj.Namespace {
					namespaceKeys = append(namespaceKeys, key)
				}
			}
		}

		var newPodSet []core_v1.Pod
		for _, namespaceKey := range namespaceKeys {
			metaIdentifierString := strings.Split(namespaceKey, "$")[1]
			metaIdentifier := MetaIdentifier{}

			json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

			for _, podChileSet := range metaIdentifier.PodsIdentifier {
				if podChileSet.Name == obj.Name {
					//Pod is being deleted.
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					newPodSet = nil
					for _, mappedPod := range mappedResource.Kube.Pods {
						if mappedPod.Name != obj.Name {
							newPodSet = append(newPodSet, mappedPod)
						}
					}

					if len(mappedResource.Kube.Ingresses) > 0 || len(mappedResource.Kube.Services) > 0 || len(mappedResource.Kube.Deployments) > 0 || len(mappedResource.Kube.ReplicaSets) > 0 || len(mappedResource.Kube.Pods) > 1 {
						//It has another resources.
						mappedResource.Kube.Pods = nil
						mappedResource.Kube.Pods = newPodSet

						m.info(fmt.Sprintf("\nDELETE Completed. - \n K8s Type - %s\n Name - %s\n Namespace - %s\nCL updated.\n", obj.ResourceType, obj.Name, obj.Namespace))
						return MapResult{
							Action:         "Updated",
							Key:            namespaceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
							Message:        fmt.Sprintf("Pod %s is deleted from Common Label %s", pod.Name, mappedResource.CommonLabel),
						}, nil
					}

					m.info(fmt.Sprintf("\nDELETE Completed. - \n K8s Type - %s\n Name - %s\n Namespace - %s\nCL deleted.\n", obj.ResourceType, obj.Name, obj.Namespace))
					return MapResult{
						Action:         "Deleted",
						Key:            namespaceKey,
						IsMapped:       true,
						CommonLabel:    mappedResource.CommonLabel,
						MappedResource: mappedResource,
						Message:        fmt.Sprintf("Pod %s is deleted from Common Label %s", pod.Name, mappedResource.CommonLabel),
					}, nil

				}
			}
		}
	}
	return MapResult{}, nil
}

func (m *Mapper) mapReplicaSetObj(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var replicaSet ext_v1beta1.ReplicaSet
	var namespaceKeys []string

	if obj.Event != nil {
		replicaSet = *obj.Event.(*ext_v1beta1.ReplicaSet).DeepCopy()

		keys := store.ListKeys()
		for _, b64Key := range keys {
			encodedKey, _ := base64.StdEncoding.DecodeString(b64Key)
			key := fmt.Sprintf("%s", encodedKey)
			if len(strings.Split(key, "$")) > 0 {
				if strings.Split(key, "$")[0] == obj.Namespace {
					namespaceKeys = append(namespaceKeys, key)
				}
			}
		}

		for _, namespaceKey := range namespaceKeys {
			metaIdentifierString := strings.Split(namespaceKey, "$")[1]
			metaIdentifier := MetaIdentifier{}

			json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

			//Try matching with Service
			if metaIdentifier.ServicesIdentifier.MatchLabels != nil {
				for _, svcID := range metaIdentifier.ServicesIdentifier.MatchLabels {
					rsMatchedLabels := make(map[string]string)
					if svcID != nil && replicaSet.Spec.Selector.MatchLabels != nil {
						for svcKey, svcValue := range svcID {
							if val, ok := replicaSet.Spec.Selector.MatchLabels[svcKey]; ok {
								if val == svcValue {
									rsMatchedLabels[svcKey] = svcValue
								}
							}
						}
					}
					if reflect.DeepEqual(rsMatchedLabels, svcID) {
						//Service and pod matches. Add pod to this mapped resource
						// mappedResource, _ := getObjectFromStore(namespaceKey, store)
						mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

						for i, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
							if mappedReplicaSet.Name == replicaSet.Name {
								mappedResource.Kube.ReplicaSets[i] = replicaSet

								return MapResult{
									Action:         "Updated",
									Key:            namespaceKey,
									IsMapped:       true,
									MappedResource: mappedResource,
									Message:        fmt.Sprintf("Replica set %s is updated in Common Label %s after matching with service", replicaSet.Name, mappedResource.CommonLabel),
								}, nil
							}
						}

						mappedResource.Kube.ReplicaSets = append(mappedResource.Kube.ReplicaSets, replicaSet)

						return MapResult{
							Action:         "Updated",
							Key:            namespaceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
							Message:        fmt.Sprintf("Replica set %s is added to Common Label %s after matching with service", replicaSet.Name, mappedResource.CommonLabel),
						}, nil
					}
				}
			}

			//Try matching with Deployment
			for _, depID := range metaIdentifier.DeploymentsIdentifier.MatchLabels {
				rsMatchedLabels := make(map[string]string)
				for depKey, depValue := range depID {
					if val, ok := replicaSet.Spec.Selector.MatchLabels[depKey]; ok {
						if val == depValue {
							rsMatchedLabels[depKey] = depValue
						}
					}
				}
				if reflect.DeepEqual(rsMatchedLabels, depID) {
					//Service and deployment matches. Add service to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
						if mappedReplicaSet.Name == replicaSet.Name {
							mappedResource.Kube.ReplicaSets[i] = replicaSet

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
								Message:        fmt.Sprintf("Replica set %s is updated in Common Label %s after matching with deployment", replicaSet.Name, mappedResource.CommonLabel),
							}, nil
						}
					}

					mappedResource.Kube.ReplicaSets = append(mappedResource.Kube.ReplicaSets, replicaSet)
					return MapResult{
						Action:         "Updated",
						Key:            namespaceKey,
						IsMapped:       true,
						MappedResource: mappedResource,
						Message:        fmt.Sprintf("Replica set %s is added to Common Label %s after matching with deployment", replicaSet.Name, mappedResource.CommonLabel),
					}, nil
				}
			}

			//Try matching with Replica set
			for _, rsID := range metaIdentifier.ReplicaSetsIdentifier {
				if reflect.DeepEqual(replicaSet.Spec.Selector.MatchLabels, rsID.MatchLabels) {
					//Service and deployment matches. Add service to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
						if mappedReplicaSet.Name == replicaSet.Name {
							mappedResource.Kube.ReplicaSets[i] = replicaSet

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
								Message:        fmt.Sprintf("Replica set %s is updated io Common Label %s after matching with replica set", replicaSet.Name, mappedResource.CommonLabel),
							}, nil
						}
					}
				}
			}

			//Try matching with Pod
			for _, podID := range metaIdentifier.PodsIdentifier {
				rsMatchedLabels := make(map[string]string)
				for podKey, podValue := range podID.MatchLabels {
					if val, ok := replicaSet.Spec.Selector.MatchLabels[podKey]; ok {
						if val == podValue {
							rsMatchedLabels[podKey] = podValue
						}
					}
				}
				if reflect.DeepEqual(rsMatchedLabels, replicaSet.Spec.Selector.MatchLabels) {
					//Service and deployment matches. Add service to this mapped resource
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					for i, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
						if mappedReplicaSet.Name == replicaSet.Name {
							mappedResource.Kube.ReplicaSets[i] = replicaSet

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
								Message:        fmt.Sprintf("Replica set %s is updated in Common Label %s after matching with pod", replicaSet.Name, mappedResource.CommonLabel),
							}, nil
						}
					}

					mappedResource.Kube.ReplicaSets = append(mappedResource.Kube.ReplicaSets, replicaSet)
					if len(mappedResource.Kube.ReplicaSets) < 2 { //Set Common Label to service name.
						mappedResource.CommonLabel = replicaSet.Name
					}
					return MapResult{
						Action:         "Updated",
						Key:            namespaceKey,
						IsMapped:       true,
						MappedResource: mappedResource,
						Message:        fmt.Sprintf("Replica set %s is added to Common Label %s after matching with pod", replicaSet.Name, mappedResource.CommonLabel),
					}, nil
				}
			}
		}

		//Didn't find any match. Create new resource
		newMappedService := MappedResource{}
		newMappedService.CommonLabel = replicaSet.Name
		newMappedService.CurrentType = "replicaset"
		newMappedService.Namespace = replicaSet.Namespace
		newMappedService.Kube.ReplicaSets = append(newMappedService.Kube.ReplicaSets, replicaSet)

		return MapResult{
			Action:         "Added",
			IsMapped:       true,
			MappedResource: newMappedService,
			Message:        fmt.Sprintf("New replica set %s is added with Common Label %s", replicaSet.Name, newMappedService.CommonLabel),
		}, nil

	}

	//Handle Delete
	if obj.EventType == "DELETED" {
		m.info(fmt.Sprintf("\nDELETE received. - \n K8s Type - %s\n Name - %s\n Namespace - %s\n", obj.ResourceType, obj.Name, obj.Namespace))

		keys := store.ListKeys()
		for _, b64Key := range keys {
			encodedKey, _ := base64.StdEncoding.DecodeString(b64Key)
			key := fmt.Sprintf("%s", encodedKey)
			if len(strings.Split(key, "$")) > 0 {
				if strings.Split(key, "$")[0] == obj.Namespace {
					namespaceKeys = append(namespaceKeys, key)
				}
			}
		}

		var newRsSet []ext_v1beta1.ReplicaSet
		for _, namespaceKey := range namespaceKeys {
			metaIdentifierString := strings.Split(namespaceKey, "$")[1]
			metaIdentifier := MetaIdentifier{}

			json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

			for _, rsChileSet := range metaIdentifier.ReplicaSetsIdentifier {
				if rsChileSet.Name == obj.Name {
					//Pod is being deleted.
					// mappedResource, _ := getObjectFromStore(namespaceKey, store)
					mappedResource, _ := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(namespaceKey)), store)

					newRsSet = nil
					for _, mappedRs := range mappedResource.Kube.ReplicaSets {
						if mappedRs.Name != obj.Name {
							newRsSet = append(newRsSet, mappedRs)
						}
					}

					if len(mappedResource.Kube.Ingresses) > 0 || len(mappedResource.Kube.Services) > 0 || len(mappedResource.Kube.Deployments) > 0 || len(mappedResource.Kube.Pods) > 0 || len(mappedResource.Kube.ReplicaSets) > 1 {
						//It has another resources.
						mappedResource.Kube.ReplicaSets = nil
						mappedResource.Kube.ReplicaSets = newRsSet

						m.info(fmt.Sprintf("\nDELETE Completed. - \n K8s Type - %s\n Name - %s\n Namespace - %s\nCL updated.\n", obj.ResourceType, obj.Name, obj.Namespace))
						return MapResult{
							Action:         "Updated",
							Key:            namespaceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
							Message:        fmt.Sprintf("Replica set %s is deleted from Common Label %s", replicaSet.Name, mappedResource.CommonLabel),
						}, nil
					}

					m.info(fmt.Sprintf("\nDELETE Completed. - \n K8s Type - %s\n Name - %s\n Namespace - %s\nCL deleted.\n", obj.ResourceType, obj.Name, obj.Namespace))
					return MapResult{
						Action:         "Deleted",
						Key:            namespaceKey,
						IsMapped:       true,
						CommonLabel:    mappedResource.CommonLabel,
						MappedResource: mappedResource,
						Message:        fmt.Sprintf("Replica set %s is deleted from Common Label %s", replicaSet.Name, mappedResource.CommonLabel),
					}, nil

				}
			}
		}
	}

	return MapResult{}, nil
}
