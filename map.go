package kubemap

import (
	"fmt"
	"reflect"
	"strings"

	apps_v1beta2 "k8s.io/api/apps/v1beta2"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

func kubemap(obj interface{}, store cache.Store) ([]MapResult, error) {
	object := obj.(ResourceEvent)

	mappedResource, mapErr := resourceMap(object, store)
	if mapErr != nil {
		return []MapResult{}, mapErr
	}

	storeErr := updateStore(mappedResource, store)
	if storeErr != nil {
		return []MapResult{}, storeErr
	}

	return mappedResource, nil
}

func resourceMap(obj ResourceEvent, store cache.Store) ([]MapResult, error) {
	switch obj.ResourceType {
	case "ingress":
		mappedIngress, err := mapIngress(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return mappedIngress, nil
	case "service":
		mappedService, err := mapService(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return mappedService, nil
	case "deployment":
		mappedDeployment, err := mapDeployment(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return []MapResult{
			mappedDeployment,
		}, nil
	case "replicaset":
		mappedReplicaSet, err := mapReplicaSet(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return []MapResult{
			mappedReplicaSet,
		}, nil
	case "pod":
		mappedPod, err := mapPod(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return []MapResult{
			mappedPod,
		}, nil
	}

	return []MapResult{}, fmt.Errorf("Resource type '%s' is not supported for mapping", obj.ResourceType)
}

/*
	Ingress can either be independent resource or mapped to service
	If its an independent service then create it with type 'service'
*/
func mapIngress(obj ResourceEvent, store cache.Store) ([]MapResult, error) {
	var ingress ext_v1beta1.Ingress

	if obj.Event != nil {
		ingress = *obj.Event.(*ext_v1beta1.Ingress).DeepCopy()
	}

	if obj.EventType == "ADDED" {
		return addIngress(ingress, obj, store)
	}

	if obj.EventType == "UPDATED" {
		var mapResult []MapResult
		delResults, delErr := deleteIngress(obj, store)
		if delErr != nil {
			return mapResult, delErr
		}

		storeErr := updateStore(delResults, store)
		if storeErr != nil {
			return []MapResult{}, storeErr
		}

		for _, delResult := range delResults {
			delResult.IsStoreUpdated = true
			mapResult = append(mapResult, delResult)
		}

		addResults, addErr := addIngress(ingress, obj, store)
		if addErr != nil {
			return mapResult, addErr
		}

		storeErr = updateStore(addResults, store)
		if storeErr != nil {
			return []MapResult{}, storeErr
		}

		for _, addResult := range addResults {
			addResult.IsStoreUpdated = true
			mapResult = append(mapResult, addResult)
		}

		return mapResult, nil
	}

	if obj.EventType == "DELETED" {
		return deleteIngress(obj, store)
	}

	return []MapResult{}, fmt.Errorf("Only Events of type ADDED, UPDATED and DELETED are supported. Event type '%s' is not supported", obj.EventType)
}

func addIngress(ingress ext_v1beta1.Ingress, obj ResourceEvent, store cache.Store) ([]MapResult, error) {
	var serviceMappingResults []MapResult
	var err error
	var ingressBackendServices []string
	var mapResults []MapResult

	//Get default backed service
	if ingress.Spec.Backend != nil {
		ingressBackendServices = append(ingressBackendServices, ingress.Spec.Backend.ServiceName)
	}

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

	for _, ingressBackendService := range ingressBackendServices {
		//Try matching with service
		serviceMappingResults, err = serviceMatching(obj, store, ingressBackendService)
		if err != nil {
			return []MapResult{}, err
		}

		for _, serviceMappingResult := range serviceMappingResults {
			if serviceMappingResult.IsMapped {
				mapResults = append(mapResults, serviceMappingResult)
			} else {
				//Create ingress as new mapped resource of type 'service'

				newMappedIngressService := MappedResource{}
				newMappedIngressService.CommonLabel = ingress.Name
				newMappedIngressService.CurrentType = "service"
				newMappedIngressService.Namespace = ingress.Namespace
				newMappedIngressService.Kube.Ingresses = append(newMappedIngressService.Kube.Ingresses, ingress)

				result := MapResult{
					Action:         "Added",
					IsMapped:       true,
					MappedResource: newMappedIngressService,
				}
				mapResults = append(mapResults, result)
			}
		}
	}
	return mapResults, nil
}

func deleteIngress(obj ResourceEvent, store cache.Store) ([]MapResult, error) {
	var mapResults []MapResult

	//DELETE Ingress
	//Get all services
	var ingressSvcKeys []string
	keys := store.ListKeys()
	for _, key := range keys {
		//To update lone pod. get them by exact name
		if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
			ingressSvcKeys = append(ingressSvcKeys, key)
		}
	}

	var newIngressSet []ext_v1beta1.Ingress
	if len(ingressSvcKeys) > 0 {
		for _, ingressSvcKey := range ingressSvcKeys {
			newIngressSet = nil
			mappedResource, err := getObjectFromStore(ingressSvcKey, store)
			if err != nil {
				return []MapResult{}, err
			}

			if len(mappedResource.Kube.Ingresses) > 0 {
				isMatched := false
				for _, mappedIngress := range mappedResource.Kube.Ingresses {
					if fmt.Sprintf("%s", mappedIngress.UID) == obj.UID {
						isMatched = true
					} else {
						newIngressSet = append(newIngressSet, mappedIngress)
					}
					// if fmt.Sprintf("%s", mappedIngress.UID) != obj.UID {
					// 	newIngressSet = append(newIngressSet, mappedIngress)
					// }
				}

				if isMatched {
					if len(mappedResource.Kube.Services) > 0 || len(mappedResource.Kube.Deployments) > 0 || len(mappedResource.Kube.ReplicaSets) > 0 || len(mappedResource.Kube.Pods) > 0 {
						//It has another resources
						mappedResource.Kube.Ingresses = nil
						mappedResource.Kube.Ingresses = newIngressSet

						// //Update Common Label
						// if len(mappedResource.Kube.Ingresses) > 0 {
						// 	var ingressNames []string
						// 	for _, mappedIngress := range mappedResource.Kube.Ingresses {
						// 		ingressNames = append(ingressNames, mappedIngress.Name)
						// 	}
						// 	commonLabel := strings.Join(ingressNames, "-")
						// 	if commonLabel != mappedResource.CommonLabel {
						// 		mappedResource.CommonLabel = strings.Join(ingressNames, "-")

						// 		//Since we updated the common label. Delete old resource from local store
						// 		deleteResource := MapResult{
						// 			Action:         "ForceDeleted",
						// 			Key:            ingressSvcKey,
						// 			IsMapped:       true,
						// 			IsStoreUpdated: true, //Make it as store updated so it will be just sent as an event
						// 			MappedResource: copyOfMappedResource,
						// 			CommonLabel:    copyOfMappedResource.CommonLabel,
						// 		}

						// 		mapResults = append(mapResults, deleteResource)
						// 	}

						// 	result := MapResult{
						// 		Action:         "Updated",
						// 		Key:            ingressSvcKey,
						// 		IsMapped:       true,
						// 		MappedResource: mappedResource,
						// 	}

						// 	mapResults = append(mapResults, result)
						// }
						result := MapResult{
							Action:         "Updated",
							Key:            ingressSvcKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}

						mapResults = append(mapResults, result)
						return mapResults, nil
					}
					// //Ingress is not available in newly mapped resource
					// //Update CL with service name
					// var serviceNames []string
					// for _, newMappedService := range mappedResource.Kube.Services {
					// 	serviceNames = append(serviceNames, newMappedService.Name)
					// }

					// mappedResource.CommonLabel = strings.Join(serviceNames, ", ")

					//It has just Ingress, but more than one.
					if len(mappedResource.Kube.Ingresses) > 1 {
						mappedResource.Kube.Ingresses = nil
						mappedResource.Kube.Ingresses = newIngressSet

						result := MapResult{
							Action:      "Updated",
							Key:         ingressSvcKey,
							CommonLabel: mappedResource.CommonLabel,
							IsMapped:    true,
						}

						mapResults = append(mapResults, result)

						return mapResults, nil
					}

					//If it has only one ingress. newIngressSet will be nil
					mappedResource.Kube.Ingresses = nil
					mappedResource.Kube.Ingresses = newIngressSet

					result := MapResult{
						Action:      "Deleted",
						Key:         ingressSvcKey,
						CommonLabel: mappedResource.CommonLabel,
						IsMapped:    true,
					}

					mapResults = append(mapResults, result)

					return mapResults, nil

				}
			}
		}
	}
	return []MapResult{}, nil
}

/*
	Service can either be independent resource or mapped to ingress
	Service can also be mapped to individual existing deployment/s, replica set/s or pod/s from store.
*/
func mapService(obj ResourceEvent, store cache.Store) ([]MapResult, error) {
	var service core_v1.Service
	var podMappingResult, replicaSetMappingResult, deploymentMappingResult MapResult
	var serviceMappingResults []MapResult
	var err error

	if obj.EventType != "DELETED" {
		//Try matching with service
		serviceMappingResults, err = serviceMatching(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		if !serviceMappingResults[0].IsMapped {
			deploymentMappingResult, err = deploymentMatching(obj, store)
			if err != nil {
				return []MapResult{}, err
			}

			if !deploymentMappingResult.IsMapped {
				//Try matching with replica set
				replicaSetMappingResult, err = replicaSetMatching(obj, store)
				if err != nil {
					return []MapResult{}, err
				}

				//Try matching with any individual replica set
				if !replicaSetMappingResult.IsMapped {
					podMappingResult, err = podMatching(obj, store)
					if err != nil {
						return []MapResult{}, err
					}

					if !podMappingResult.IsMapped {
						//Service not mapped with any existing mapped resources. Create as an individual resource
						if obj.Event != nil {
							service = *obj.Event.(*core_v1.Service).DeepCopy()
						}

						mappedIndividualResource := MappedResource{}
						mappedIndividualResource.CommonLabel = service.Name
						mappedIndividualResource.CurrentType = "service"
						mappedIndividualResource.Namespace = service.Namespace
						mappedIndividualResource.Kube.Services = append(mappedIndividualResource.Kube.Services, service)

						return []MapResult{
							MapResult{
								Action:         "Added",
								IsMapped:       true,
								MappedResource: mappedIndividualResource,
							},
						}, nil
					}

					return []MapResult{
						podMappingResult,
					}, nil
				}

				return []MapResult{
					replicaSetMappingResult,
				}, nil
			}

			return []MapResult{
				deploymentMappingResult,
			}, nil
		}

		return serviceMappingResults, nil
	}

	//Service is DELETED

	//Try deleting lone service
	//get lone svc from store
	var loneSvcKeys []string
	keys := store.ListKeys()
	for _, key := range keys {
		//To update lone pod. get them by exact name
		if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
			loneSvcKeys = append(loneSvcKeys, key)
		}
	}

	if len(loneSvcKeys) > 0 {
		//Delete that single pod.
		for _, loneSvcKey := range loneSvcKeys {
			var newSvcSet []core_v1.Service
			isMatched := false

			mappedResource, err := getObjectFromStore(loneSvcKey, store)
			if err != nil {
				return []MapResult{}, err
			}

			if len(mappedResource.Kube.Services) > 0 {
				for _, mappedService := range mappedResource.Kube.Services {
					if fmt.Sprintf("%s", mappedService.UID) == obj.UID {
						isMatched = true
					} else {
						newSvcSet = append(newSvcSet, mappedService)
					}
				}

				if isMatched {
					if len(mappedResource.Kube.Ingresses) > 0 || len(mappedResource.Kube.Deployments) > 0 || len(mappedResource.Kube.ReplicaSets) > 0 || len(mappedResource.Kube.Pods) > 0 {
						//It has another resources
						mappedResource.Kube.Services = nil
						mappedResource.Kube.Services = newSvcSet

						return []MapResult{
							MapResult{
								Action:         "Updated",
								Key:            loneSvcKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							},
						}, nil
					}
					return []MapResult{
						MapResult{
							Action:      "Deleted",
							Key:         loneSvcKey,
							CommonLabel: mappedResource.CommonLabel,
							IsMapped:    true,
						},
					}, nil
				}
			}
			newSvcSet = nil
			isMatched = false
		}
	}

	return []MapResult{}, fmt.Errorf("Only Events of type ADDED, UPDATED and DELETED are supported. Event type '%s' is not supported", obj.EventType)
}

/*
	Deployment can either be independent resource or mapped to service
	Deployment can also be mapped to individual existing pod/s or replica set/s, from store.
*/
func mapDeployment(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var deployment apps_v1beta2.Deployment
	var podMappingResult, replicaSetMappingResult, deploymentMappingResult MapResult
	var serviceMappingResults []MapResult
	var err error

	if obj.EventType != "DELETED" {
		//Try match with itself to figure out it's an update
		deploymentMappingResult, err = deploymentMatching(obj, store)
		if err != nil {
			return MapResult{}, err
		}

		if !deploymentMappingResult.IsMapped {
			//Try matching with service
			serviceMappingResults, err = serviceMatching(obj, store)
			if err != nil {
				return MapResult{}, err
			}

			if !serviceMappingResults[0].IsMapped {
				//Try matching with any individual replica set
				replicaSetMappingResult, err = replicaSetMatching(obj, store)
				if err != nil {
					return MapResult{}, err
				}

				//Try matching with any individual replica set
				if !replicaSetMappingResult.IsMapped {
					podMappingResult, err = podMatching(obj, store)
					if err != nil {
						return MapResult{}, err
					}

					if !podMappingResult.IsMapped {
						//Deployment not mapped with any existing mapped resources. Create as an individual resource
						if obj.Event != nil {
							deployment = *obj.Event.(*apps_v1beta2.Deployment).DeepCopy()
						}

						mappedIndividualResource := MappedResource{}
						mappedIndividualResource.CommonLabel = deployment.Name
						mappedIndividualResource.CurrentType = "deployment"
						mappedIndividualResource.Namespace = deployment.Namespace
						mappedIndividualResource.Kube.Deployments = append(mappedIndividualResource.Kube.Deployments, deployment)

						return MapResult{
							Action:         "Added",
							Message:        fmt.Sprintf("Deployment with common label '%s' is added in store", mappedIndividualResource.CommonLabel),
							IsMapped:       true,
							MappedResource: mappedIndividualResource,
						}, nil
					}

					return podMappingResult, nil
				}

				return replicaSetMappingResult, nil
			}

			return serviceMappingResults[0], nil
		}

		return deploymentMappingResult, nil
	}

	//Deployment is DELETED

	//Try deleting lone Dep
	//get lone Dep from store
	var loneDepKeys []string
	keys := store.ListKeys()
	for _, key := range keys {
		//To update lone pod. get them by exact name
		if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "deployment" {
			loneDepKeys = append(loneDepKeys, key)
		}
	}

	if len(loneDepKeys) > 0 {
		//Delete that single pod.
		for _, loneDepKey := range loneDepKeys {
			var newDepSet []apps_v1beta2.Deployment
			isMatched := false

			mappedResource, err := getObjectFromStore(loneDepKey, store)
			if err != nil {
				return MapResult{}, err
			}

			if len(mappedResource.Kube.Deployments) > 0 {
				for _, mappedDeployment := range mappedResource.Kube.Deployments {
					if fmt.Sprintf("%s", mappedDeployment.UID) == obj.UID {
						isMatched = true
					} else {
						newDepSet = append(newDepSet, mappedDeployment)
					}
				}

				if isMatched {
					if len(mappedResource.Kube.ReplicaSets) > 0 || len(mappedResource.Kube.Pods) > 0 {
						//It has another resources
						mappedResource.Kube.Deployments = nil
						mappedResource.Kube.Deployments = newDepSet

						return MapResult{
							Action:         "Updated",
							Key:            loneDepKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					} else if len(mappedResource.Kube.Deployments) > 1 {
						mappedResource.Kube.Deployments = nil
						mappedResource.Kube.Deployments = newDepSet

						return MapResult{
							Action:         "Updated",
							Key:            loneDepKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
					return MapResult{
						Action:      "Deleted",
						Key:         loneDepKey,
						CommonLabel: mappedResource.CommonLabel,
						IsMapped:    true,
					}, nil
				}
			}
			newDepSet = nil
			isMatched = false
		}
	}

	//Try deleting Deployment mapped to Service
	//get service from store
	var loneDepSvcKeys []string
	depSvcKeys := store.ListKeys()
	for _, key := range depSvcKeys {
		//To update lone pod. get them by exact name
		if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
			loneDepSvcKeys = append(loneDepSvcKeys, key)
		}
	}

	if len(loneDepSvcKeys) > 0 {
		//Delete that single deployment.
		for _, loneDepSvcKey := range loneDepSvcKeys {
			var newDepSet []apps_v1beta2.Deployment
			isMatched := false
			mappedResource, err := getObjectFromStore(loneDepSvcKey, store)
			if err != nil {
				return MapResult{}, err
			}

			for _, mappedDeployment := range mappedResource.Kube.Deployments {
				if fmt.Sprintf("%s", mappedDeployment.UID) == obj.UID {
					isMatched = true
				} else {
					newDepSet = append(newDepSet, mappedDeployment)
				}
			}

			if isMatched {
				if len(mappedResource.Kube.Ingresses) > 0 || len(mappedResource.Kube.Services) > 0 || len(mappedResource.Kube.ReplicaSets) > 0 || len(mappedResource.Kube.Pods) > 0 {
					//It has another resources
					mappedResource.Kube.Deployments = nil
					mappedResource.Kube.Deployments = newDepSet

					return MapResult{
						Action:         "Updated",
						Key:            loneDepSvcKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				} else if len(mappedResource.Kube.Deployments) > 1 {
					//It has more than one pod
					mappedResource.Kube.Deployments = nil
					mappedResource.Kube.Deployments = newDepSet

					return MapResult{
						Action:         "Updated",
						Key:            loneDepSvcKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				} else {
					return MapResult{
						Action:      "Deleted",
						Key:         loneDepSvcKey,
						CommonLabel: mappedResource.CommonLabel,
						IsMapped:    true,
					}, nil
				}
			}
			newDepSet = nil
			isMatched = false
		}
	}

	return MapResult{}, fmt.Errorf("Only Events of type ADDED, UPDATED and DELETED are supported. Event type '%s' is not supported", obj.EventType)
}

/*
	Replica Set can either be independent resource or mapped to deployment or service
	RS can also be mapped to individual existing pod from store.
*/
func mapReplicaSet(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var replicaSet ext_v1beta1.ReplicaSet
	var podMappingResult, deploymentMappingResult, replicaSetMappingResult MapResult
	var serviceMappingResults []MapResult
	var err error

	if obj.EventType != "DELETED" {
		//Try matching with itself
		replicaSetMappingResult, err = replicaSetMatching(obj, store)
		if !replicaSetMappingResult.IsMapped {

			//Try matching with deployment
			deploymentMappingResult, err = deploymentMatching(obj, store)
			if err != nil {
				return MapResult{}, err
			}

			//Try matching with service
			if !deploymentMappingResult.IsMapped {
				serviceMappingResults, err = serviceMatching(obj, store)
				if err != nil {
					return MapResult{}, err
				}

				//Try matching with any individual pod
				if !serviceMappingResults[0].IsMapped {
					podMappingResult, err = podMatching(obj, store)
					if err != nil {
						return MapResult{}, err
					}

					if !podMappingResult.IsMapped {
						//RS not mapped with any existing mapped resources. Create as an individual resource
						if obj.Event != nil {
							replicaSet = *obj.Event.(*ext_v1beta1.ReplicaSet).DeepCopy()
						}

						mappedIndividualResource := MappedResource{}
						mappedIndividualResource.CommonLabel = replicaSet.Name
						mappedIndividualResource.CurrentType = "replicaset"
						mappedIndividualResource.Namespace = replicaSet.Namespace
						mappedIndividualResource.Kube.ReplicaSets = append(mappedIndividualResource.Kube.ReplicaSets, replicaSet)

						return MapResult{
							Action:         "Added",
							IsMapped:       true,
							MappedResource: mappedIndividualResource,
						}, nil
					}

					return podMappingResult, nil
				}
				return serviceMappingResults[0], nil

			}

			return deploymentMappingResult, nil
		}

		return replicaSetMappingResult, nil
	}

	//RS is DELETED

	//Try deleting lone RS
	//get lone RS from store
	var loneRsKeys []string
	keys := store.ListKeys()
	for _, key := range keys {
		//To update lone pod. get them by exact name
		if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "replicaset" {
			loneRsKeys = append(loneRsKeys, key)
		}
	}

	if len(loneRsKeys) > 0 {
		//Delete that single pod.
		for _, loneRsKey := range loneRsKeys {
			var newRsSet []ext_v1beta1.ReplicaSet
			isMatched := false

			mappedResource, err := getObjectFromStore(loneRsKey, store)
			if err != nil {
				return MapResult{}, err
			}

			if len(mappedResource.Kube.ReplicaSets) > 0 {
				for _, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
					if fmt.Sprintf("%s", mappedReplicaSet.UID) == obj.UID {
						isMatched = true
					} else {
						newRsSet = append(newRsSet, mappedReplicaSet)
					}
				}

				if isMatched {
					if len(mappedResource.Kube.Pods) > 0 {
						//It has another resources
						mappedResource.Kube.ReplicaSets = nil
						mappedResource.Kube.ReplicaSets = newRsSet

						return MapResult{
							Action:         "Updated",
							Key:            loneRsKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					} else if len(mappedResource.Kube.ReplicaSets) > 1 {
						mappedResource.Kube.ReplicaSets = nil
						mappedResource.Kube.ReplicaSets = newRsSet

						return MapResult{
							Action:         "Updated",
							Key:            loneRsKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
					return MapResult{
						Action:      "Deleted",
						Key:         loneRsKey,
						CommonLabel: mappedResource.CommonLabel,
						IsMapped:    true,
					}, nil

				}
			}
			newRsSet = nil
			isMatched = false
		}
	}

	//Try deleting RS mapped to Deployment
	//get pod from store
	var loneRsDepKeys []string
	rsDepKeys := store.ListKeys()
	for _, key := range rsDepKeys {
		//To update lone pod. get them by exact name
		if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "deployment" {
			loneRsDepKeys = append(loneRsDepKeys, key)
		}
	}

	if len(loneRsDepKeys) > 0 {
		//Delete that single pod.
		for _, loneRsDepKey := range loneRsDepKeys {
			var newRsSet []ext_v1beta1.ReplicaSet
			isMatched := false
			mappedResource, err := getObjectFromStore(loneRsDepKey, store)
			if err != nil {
				return MapResult{}, err
			}

			for _, mappedRs := range mappedResource.Kube.ReplicaSets {
				if fmt.Sprintf("%s", mappedRs.UID) == obj.UID {
					isMatched = true
				} else {
					newRsSet = append(newRsSet, mappedRs)
				}
			}

			if isMatched {
				if len(mappedResource.Kube.Deployments) > 0 || len(mappedResource.Kube.Pods) > 0 {
					//It has another resources
					mappedResource.Kube.ReplicaSets = nil
					mappedResource.Kube.ReplicaSets = newRsSet

					return MapResult{
						Action:         "Updated",
						Key:            loneRsDepKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				} else if len(mappedResource.Kube.ReplicaSets) > 1 {
					//It has more than one pod
					mappedResource.Kube.ReplicaSets = nil
					mappedResource.Kube.ReplicaSets = newRsSet

					return MapResult{
						Action:         "Updated",
						Key:            loneRsDepKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				} else {
					return MapResult{
						Action:      "Deleted",
						Key:         loneRsDepKey,
						CommonLabel: mappedResource.CommonLabel,
						IsMapped:    true,
					}, nil
				}
			}
			newRsSet = nil
			isMatched = false
		}
	}

	//Try deleting RS mapped to Service
	//get pod from store
	var loneRsSvcKeys []string
	rsSvcKeys := store.ListKeys()
	for _, key := range rsSvcKeys {
		//To update lone pod. get them by exact name
		if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
			loneRsSvcKeys = append(loneRsSvcKeys, key)
		}
	}

	if len(loneRsSvcKeys) > 0 {
		//Delete that single pod.
		for _, loneRsSvcKey := range loneRsSvcKeys {
			var newRsSet []ext_v1beta1.ReplicaSet
			isMatched := false
			mappedResource, err := getObjectFromStore(loneRsSvcKey, store)
			if err != nil {
				return MapResult{}, err
			}

			for _, mappedPod := range mappedResource.Kube.ReplicaSets {
				if fmt.Sprintf("%s", mappedPod.UID) == obj.UID {
					isMatched = true
				} else {
					newRsSet = append(newRsSet, mappedPod)
				}
			}

			if isMatched {
				if len(mappedResource.Kube.Ingresses) > 0 || len(mappedResource.Kube.Services) > 0 || len(mappedResource.Kube.Deployments) > 0 || len(mappedResource.Kube.Pods) > 0 {
					//It has another resources
					mappedResource.Kube.ReplicaSets = nil
					mappedResource.Kube.ReplicaSets = newRsSet

					return MapResult{
						Action:         "Updated",
						Key:            loneRsSvcKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				} else if len(mappedResource.Kube.ReplicaSets) > 1 {
					//It has more than one pod
					mappedResource.Kube.ReplicaSets = nil
					mappedResource.Kube.ReplicaSets = newRsSet

					return MapResult{
						Action:         "Updated",
						Key:            loneRsSvcKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				} else {
					return MapResult{
						Action:      "Deleted",
						Key:         loneRsSvcKey,
						CommonLabel: mappedResource.CommonLabel,
						IsMapped:    true,
					}, nil
				}
			}
			newRsSet = nil
			isMatched = false
		}
	}

	return MapResult{}, fmt.Errorf("Only Events of type ADDED, UPDATED and DELETED are supported. Event type '%s' is not supported", obj.EventType)
}

/*
	Pod can either be independent resource or mapped to rs, deployment or service
*/
func mapPod(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var pod core_v1.Pod
	var rsMappingResult, deploymentMappingResult, podMappingResult MapResult
	var serviceMappingResults []MapResult
	var err error

	if obj.EventType != "DELETED" {

		//Try to map with itself to Update
		podMappingResult, err = podMatching(obj, store)
		if err != nil {
			return MapResult{}, err
		}

		if !podMappingResult.IsMapped {

			rsMappingResult, err = replicaSetMatching(obj, store)
			if err != nil {
				return MapResult{}, err
			}

			if !rsMappingResult.IsMapped {
				deploymentMappingResult, err = deploymentMatching(obj, store)
				if err != nil {
					return MapResult{}, err
				}

				if !deploymentMappingResult.IsMapped {
					serviceMappingResults, err = serviceMatching(obj, store)
					if err != nil {
						return MapResult{}, err
					}

					if !serviceMappingResults[0].IsMapped {
						//It's an individual pod. Create it
						if obj.Event != nil {
							pod = *obj.Event.(*core_v1.Pod).DeepCopy()
						}

						mappedIndividualResource := MappedResource{}
						mappedIndividualResource.CommonLabel = pod.Name
						mappedIndividualResource.CurrentType = "pod"
						mappedIndividualResource.Namespace = pod.Namespace
						mappedIndividualResource.Kube.Pods = append(mappedIndividualResource.Kube.Pods, pod)

						return MapResult{
							Action:         "Added",
							IsMapped:       true,
							MappedResource: mappedIndividualResource,
						}, nil
					}

					return serviceMappingResults[0], nil
				}

				return deploymentMappingResult, nil
			}

			return rsMappingResult, nil
		}

		return podMappingResult, nil
	}

	//Pod is DELETED

	//Try deleting lone pod
	//get lone pod from store
	var lonePodKeys []string
	keys := store.ListKeys()
	for _, key := range keys {
		//To update lone pod. get them by exact name
		if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "pod" {
			lonePodKeys = append(lonePodKeys, key)
		}
	}

	if len(lonePodKeys) > 0 {
		//Delete that single pod.
		for _, lonePodKey := range lonePodKeys {
			var newPodSet []core_v1.Pod
			isMatched := false

			mappedResource, err := getObjectFromStore(lonePodKey, store)
			if err != nil {
				return MapResult{}, err
			}

			if len(mappedResource.Kube.Pods) > 0 {
				for _, mappedPod := range mappedResource.Kube.Pods {
					if fmt.Sprintf("%s", mappedPod.UID) == obj.UID {
						isMatched = true
					} else {
						newPodSet = append(newPodSet, mappedPod)
					}
				}

				if isMatched {
					if len(mappedResource.Kube.Pods) > 1 {
						mappedResource.Kube.Pods = nil
						mappedResource.Kube.Pods = newPodSet

						return MapResult{
							Action:         "Updated",
							Key:            lonePodKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
					return MapResult{
						Action:      "Deleted",
						Key:         lonePodKey,
						CommonLabel: mappedResource.CommonLabel,
						IsMapped:    true,
					}, nil

				}
			}
		}
	}

	//Try deleting pod mapped to RS
	//get pod from store
	var lonePodRsKeys []string
	podRsKeys := store.ListKeys()
	for _, key := range podRsKeys {
		//To update lone pod. get them by exact name
		if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "replicaset" {
			lonePodRsKeys = append(lonePodRsKeys, key)
		}
	}

	if len(lonePodRsKeys) > 0 {
		//Delete that single pod.
		for _, lonePodRsKey := range lonePodRsKeys {
			var newPodSet []core_v1.Pod
			isMatched := false
			mappedResource, err := getObjectFromStore(lonePodRsKey, store)
			if err != nil {
				return MapResult{}, err
			}

			for _, mappedPod := range mappedResource.Kube.Pods {
				if fmt.Sprintf("%s", mappedPod.UID) == obj.UID {
					isMatched = true
				} else {
					newPodSet = append(newPodSet, mappedPod)
				}
			}

			if isMatched {
				if len(mappedResource.Kube.Ingresses) > 0 || len(mappedResource.Kube.Services) > 0 || len(mappedResource.Kube.Deployments) > 0 || len(mappedResource.Kube.ReplicaSets) > 0 {
					//It has another resources
					mappedResource.Kube.Pods = nil
					mappedResource.Kube.Pods = newPodSet

					return MapResult{
						Action:         "Updated",
						Key:            lonePodRsKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				} else if len(mappedResource.Kube.Pods) > 1 {
					//It has more than one pod
					mappedResource.Kube.Pods = nil
					mappedResource.Kube.Pods = newPodSet

					return MapResult{
						Action:         "Updated",
						Key:            lonePodRsKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				} else {
					return MapResult{
						Action:      "Deleted",
						Key:         lonePodRsKey,
						CommonLabel: mappedResource.CommonLabel,
						IsMapped:    true,
					}, nil
				}
			}
			newPodSet = nil
			isMatched = false
		}
	}

	//Try deleting pod mapped to Deployment
	//get pod from store
	var lonePodDepKeys []string
	podDepKeys := store.ListKeys()
	for _, key := range podDepKeys {
		//To update lone pod. get them by exact name
		if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "deployment" {
			lonePodDepKeys = append(lonePodDepKeys, key)
		}
	}

	if len(lonePodDepKeys) > 0 {
		//Delete that single pod.
		for _, lonePodDepKey := range lonePodDepKeys {
			var newPodSet []core_v1.Pod
			isMatched := false
			mappedResource, err := getObjectFromStore(lonePodDepKey, store)
			if err != nil {
				return MapResult{}, err
			}

			for _, mappedPod := range mappedResource.Kube.Pods {
				if fmt.Sprintf("%s", mappedPod.UID) == obj.UID {
					isMatched = true
				} else {
					newPodSet = append(newPodSet, mappedPod)
				}
			}

			if isMatched {
				if len(mappedResource.Kube.Ingresses) > 0 || len(mappedResource.Kube.Services) > 0 || len(mappedResource.Kube.Deployments) > 0 || len(mappedResource.Kube.ReplicaSets) > 0 {
					//It has another resources
					mappedResource.Kube.Pods = nil
					mappedResource.Kube.Pods = newPodSet

					return MapResult{
						Action:         "Updated",
						Key:            lonePodDepKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				} else if len(mappedResource.Kube.Pods) > 1 {
					//It has more than one pod
					mappedResource.Kube.Pods = nil
					mappedResource.Kube.Pods = newPodSet

					return MapResult{
						Action:         "Updated",
						Key:            lonePodDepKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				} else {
					return MapResult{
						Action:      "Deleted",
						Key:         lonePodDepKey,
						CommonLabel: mappedResource.CommonLabel,
						IsMapped:    true,
					}, nil
				}
			}
			newPodSet = nil
			isMatched = false
		}
	}

	//Try deleting pod mapped to Service
	//get pod from store
	var lonePodSvcKeys []string
	podSvcKeys := store.ListKeys()
	for _, key := range podSvcKeys {
		//To update lone pod. get them by exact name
		if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
			lonePodSvcKeys = append(lonePodSvcKeys, key)
		}
	}

	if len(lonePodSvcKeys) > 0 {
		//Delete that single pod.
		for _, lonePodSvcKey := range lonePodSvcKeys {
			var newPodSet []core_v1.Pod
			isMatched := false
			mappedResource, err := getObjectFromStore(lonePodSvcKey, store)
			if err != nil {
				return MapResult{}, err
			}

			for _, mappedPod := range mappedResource.Kube.Pods {
				if fmt.Sprintf("%s", mappedPod.UID) == obj.UID {
					isMatched = true
				} else {
					newPodSet = append(newPodSet, mappedPod)
				}
			}

			if isMatched {
				if len(mappedResource.Kube.Ingresses) > 0 || len(mappedResource.Kube.Services) > 0 || len(mappedResource.Kube.Deployments) > 0 || len(mappedResource.Kube.ReplicaSets) > 0 {
					//It has another resources
					mappedResource.Kube.Pods = nil
					mappedResource.Kube.Pods = newPodSet

					return MapResult{
						Action:         "Updated",
						Key:            lonePodSvcKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				} else if len(mappedResource.Kube.Pods) > 1 {
					//It has more than one pod
					mappedResource.Kube.Pods = nil
					mappedResource.Kube.Pods = newPodSet

					return MapResult{
						Action:         "Updated",
						Key:            lonePodSvcKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				} else {
					return MapResult{
						Action:      "Deleted",
						Key:         lonePodSvcKey,
						CommonLabel: mappedResource.CommonLabel,
						IsMapped:    true,
					}, nil
				}
			}
			newPodSet = nil
			isMatched = false
		}
	}

	return MapResult{}, fmt.Errorf("Only Events of type ADDED, UPDATED and DELETEd are supported. Event type '%s' is not supported", obj.EventType)
}

//Pod Matching
func podMatching(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var podKeys []string

	//Find matching rs for object to be matched
	switch obj.Event.(type) {
	case *ext_v1beta1.Ingress:
	case *core_v1.Service:
		//Find any individual pods matching this service
		var service core_v1.Service

		//get services from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with deployment then it must be created via deployment => RS => pod (Pod name derived from deployment)
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "pod" {
				podKeys = append(podKeys, key)
			}
		}

		if obj.Event != nil {
			service = *obj.Event.(*core_v1.Service).DeepCopy()
		}

		var deleteKeys []string
		newServiceMappedResource := MappedResource{}
		newServiceMappedResource.CommonLabel = service.Name
		newServiceMappedResource.CurrentType = "service"
		newServiceMappedResource.Namespace = service.Namespace
		newServiceMappedResource.Kube.Services = append(newServiceMappedResource.Kube.Services, service)

		for _, podKey := range podKeys {
			mappedResource, err := getObjectFromStore(podKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod matched to any of RS
			if len(mappedResource.Kube.Pods) > 0 {
				for _, pod := range mappedResource.Kube.Pods {
					podMatchedLabels := make(map[string]string)
					for svcKey, svcValue := range service.Spec.Selector {
						if val, ok := pod.Labels[svcKey]; ok {
							if val == svcValue {
								podMatchedLabels[svcKey] = svcValue
							}
						}
					}
					if reflect.DeepEqual(podMatchedLabels, service.Spec.Selector) {
						//Add service to this RS mapped resources
						newServiceMappedResource.Kube.Pods = append(newServiceMappedResource.Kube.Pods, pod)
						deleteKeys = append(deleteKeys, podKey)
					}
				}
			}
		}

		if len(deleteKeys) > 0 {
			return MapResult{
				Action:         "Added",
				DeleteKeys:     deleteKeys,
				IsMapped:       true,
				MappedResource: newServiceMappedResource,
			}, nil
		}
	case *apps_v1beta2.Deployment:
		//Find any individual pods matching this deployments
		var deployment apps_v1beta2.Deployment

		//get replica set from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with deployment then it must be created via deployment => RS => pod (Pod name derived from deployment)
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "pod" && strings.HasPrefix(strings.Split(key, "/")[2], obj.Name) {
				podKeys = append(podKeys, key)
			}
		}

		if obj.Event != nil {
			deployment = *obj.Event.(*apps_v1beta2.Deployment).DeepCopy()
		}

		newDeploymentMappedResource := MappedResource{}
		newDeploymentMappedResource.CommonLabel = deployment.Name
		newDeploymentMappedResource.CurrentType = "deployment"
		newDeploymentMappedResource.Namespace = deployment.Namespace
		newDeploymentMappedResource.Kube.Deployments = append(newDeploymentMappedResource.Kube.Deployments, deployment)

		var deleteKeys []string
		for _, podKey := range podKeys {
			mappedResource, err := getObjectFromStore(podKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod matched to any of RS
			if len(mappedResource.Kube.Pods) > 0 {
				for _, pod := range mappedResource.Kube.Pods {
					//Pod name must start with deployment name.
					if strings.HasPrefix(pod.Name, deployment.Name) {
						//pod is matched with this deployment. Add deployment to mappedResource
						newDeploymentMappedResource.Kube.Pods = append(newDeploymentMappedResource.Kube.Pods, pod)
						deleteKeys = append(deleteKeys, podKey)
					}
				}
			}
		}
		if len(deleteKeys) > 0 {
			return MapResult{
				Action:     "Updated",
				DeleteKeys: deleteKeys,
				//Message:        fmt.Sprintf("Deployment with common label '%s' is updated with Pod '%s'", newDeploymentMappedResource.CommonLabel, strings.Join(strings.Split(deleteKeys[:], "/")[2], ",")),
				IsMapped:       true,
				MappedResource: newDeploymentMappedResource,
			}, nil
		}
	case *ext_v1beta1.ReplicaSet:
		//Find any individual pods matching this replica sets
		var replicaSet ext_v1beta1.ReplicaSet

		//get replica set from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with RS then it must be created via deployment => RS => pod (Pod name derived from RS)
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "pod" && strings.HasPrefix(strings.Split(key, "/")[2], obj.Name) {
				podKeys = append(podKeys, key)
			}
		}

		if obj.Event != nil {
			replicaSet = *obj.Event.(*ext_v1beta1.ReplicaSet).DeepCopy()
		}

		//Create new RS to put matching pods in it.
		newRSMappedResource := MappedResource{}
		newRSMappedResource.CommonLabel = replicaSet.Name
		newRSMappedResource.CurrentType = "replicaset"
		newRSMappedResource.Namespace = replicaSet.Namespace
		newRSMappedResource.Kube.ReplicaSets = append(newRSMappedResource.Kube.ReplicaSets, replicaSet)

		var deleteKeys []string
		for _, podKey := range podKeys {
			mappedResource, err := getObjectFromStore(podKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod matched to any of RS
			if len(mappedResource.Kube.Pods) > 0 {
				for _, pod := range mappedResource.Kube.Pods {
					for _, podOwnerReference := range pod.OwnerReferences {
						if podOwnerReference.Name == replicaSet.Name {
							//pod is matched with this RS. Add to newRSMappedResource
							newRSMappedResource.Kube.Pods = append(newRSMappedResource.Kube.Pods, pod)
							deleteKeys = append(deleteKeys, podKey)
						}
					}
				}
			}
		}
		if len(deleteKeys) > 0 {
			return MapResult{
				Action:         "Added",
				DeleteKeys:     deleteKeys,
				IsMapped:       true,
				MappedResource: newRSMappedResource,
			}, nil
		}

	case *core_v1.Pod:
		//Find any individual pods matching this deployments
		var pod core_v1.Pod

		//get pod from store
		keys := store.ListKeys()
		for _, key := range keys {
			//To update lone pod. get them by exact name
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "pod" && strings.Split(key, "/")[2] == obj.Name {
				podKeys = append(podKeys, key)
			}
		}

		if obj.Event != nil {
			pod = *obj.Event.(*core_v1.Pod).DeepCopy()
		}

		for _, podKey := range podKeys {
			mappedResource, err := getObjectFromStore(podKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod matched to any of RS
			var newPodSet []core_v1.Pod
			if len(mappedResource.Kube.Pods) > 0 {
				for _, mappedPod := range mappedResource.Kube.Pods {
					if mappedPod.Name == pod.Name && mappedPod.UID == pod.UID {
						//Update it
						newPodSet = append(newPodSet, pod)
					} else {
						newPodSet = append(newPodSet, mappedPod)
					}
				}

				mappedResource.Kube.Pods = nil
				mappedResource.Kube.Pods = newPodSet

				return MapResult{
					Action:         "Added",
					Key:            podKey,
					IsMapped:       true,
					MappedResource: mappedResource,
				}, nil
			}
		}
	default:
		return MapResult{}, fmt.Errorf("Object %s to be mapped from namespace %s of type %s is not supported", obj.Name, obj.Namespace, obj.ResourceType)
	}

	return MapResult{
		IsMapped: false,
	}, nil
}

func deletePods(keys []string) {

}

//Replica Set Matching
func replicaSetMatching(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var rsKeys []string

	//Find matching rs for object to be matched
	switch obj.Event.(type) {
	case *core_v1.Service:
		//Find any RS matching this service
		var service core_v1.Service

		//get replica set from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with deployment then it must be created via deployment => RS => pod (Pod name derived from deployment)
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "replicaset" {
				rsKeys = append(rsKeys, key)
			}
		}

		if obj.Event != nil {
			service = *obj.Event.(*core_v1.Service).DeepCopy()
		}

		var deleteKeys []string
		newServiceMappedResource := MappedResource{}
		newServiceMappedResource.CommonLabel = service.Name
		newServiceMappedResource.CurrentType = "service"
		newServiceMappedResource.Namespace = service.Namespace
		newServiceMappedResource.Kube.Services = append(newServiceMappedResource.Kube.Services, service)

		for _, rsKey := range rsKeys {
			mappedResource, err := getObjectFromStore(rsKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if deployment is matching to any service
			if len(mappedResource.Kube.ReplicaSets) > 0 {
				for _, replicaSet := range mappedResource.Kube.ReplicaSets {
					rsMatchedLabels := make(map[string]string)
					for svcKey, svcValue := range service.Spec.Selector {
						if val, ok := replicaSet.Spec.Selector.MatchLabels[svcKey]; ok {
							if val == svcValue {
								rsMatchedLabels[svcKey] = svcValue
							}
						}
					}
					if reflect.DeepEqual(rsMatchedLabels, service.Spec.Selector) {
						//Add service to this RS mapped resources
						newServiceMappedResource.Kube.ReplicaSets = append(newServiceMappedResource.Kube.ReplicaSets, replicaSet)
						deleteKeys = append(deleteKeys, rsKey)
					}
				}
			}
		}

		if len(deleteKeys) > 0 {
			return MapResult{
				Action:         "Added",
				DeleteKeys:     deleteKeys,
				IsMapped:       true,
				MappedResource: newServiceMappedResource,
			}, nil
		}
	case *ext_v1beta1.ReplicaSet:
		//Update scenario
		var replicaSet ext_v1beta1.ReplicaSet

		if obj.Event != nil {
			replicaSet = *obj.Event.(*ext_v1beta1.ReplicaSet).DeepCopy()
		}

		//get replica set from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If deployment has to map with RS then it must be created via deployment (RS name derived from Deployment)
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "replicaset" && strings.Split(key, "/")[2] == obj.Name {
				rsKeys = append(rsKeys, key)
			}
		}

		for _, rsKey := range rsKeys {
			mappedResource, err := getObjectFromStore(rsKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod matched to any of RS
			var newRsSet []ext_v1beta1.ReplicaSet
			if len(mappedResource.Kube.ReplicaSets) > 0 {
				for _, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
					if mappedReplicaSet.Name == replicaSet.Name && mappedReplicaSet.UID == replicaSet.UID {
						//Update it
						newRsSet = append(newRsSet, replicaSet)
					} else {
						newRsSet = append(newRsSet, mappedReplicaSet)
					}
				}

				mappedResource.Kube.ReplicaSets = nil
				mappedResource.Kube.ReplicaSets = newRsSet

				return MapResult{
					Action:         "Updated",
					Message:        fmt.Sprintf("Replica set with common label '%s' is updated", mappedResource.CommonLabel),
					Key:            rsKey,
					IsMapped:       true,
					MappedResource: mappedResource,
				}, nil
			}
		}
	case *apps_v1beta2.Deployment:
		//Find any individual replica set matching this deployment
		var deployment apps_v1beta2.Deployment

		//get replica set from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If deployment has to map with RS then it must be created via deployment (RS name derived from Deployment)
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "replicaset" && strings.HasPrefix(strings.Split(key, "/")[2], obj.Name) {
				rsKeys = append(rsKeys, key)
			}
		}

		if obj.Event != nil {
			deployment = *obj.Event.(*apps_v1beta2.Deployment).DeepCopy()
		}

		var deleteKeys []string
		newDeploymentMappedResource := MappedResource{}
		newDeploymentMappedResource.CommonLabel = deployment.Name
		newDeploymentMappedResource.CurrentType = "deployment"
		newDeploymentMappedResource.Namespace = deployment.Namespace
		newDeploymentMappedResource.Kube.Deployments = append(newDeploymentMappedResource.Kube.Deployments, deployment)

		for _, rsKey := range rsKeys {
			mappedResource, err := getObjectFromStore(rsKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod matched to any of RS
			if len(mappedResource.Kube.ReplicaSets) > 0 {
				for _, replicaSet := range mappedResource.Kube.ReplicaSets {
					for _, replicaSetOwnerReference := range replicaSet.OwnerReferences {
						if replicaSetOwnerReference.Name == deployment.Name {
							//rs is matched with this Deployment. Add deployment to mappedResource
							newDeploymentMappedResource.Kube.ReplicaSets = append(newDeploymentMappedResource.Kube.ReplicaSets, replicaSet)
							deleteKeys = append(deleteKeys, rsKey)
						}
					}
				}
			}
		}
		if len(deleteKeys) > 0 {
			return MapResult{
				Action:     "Updated",
				DeleteKeys: deleteKeys,
				//Message:        fmt.Sprintf("Deployment with common label '%s' is updated with Replica sets '%s'", newDeploymentMappedResource.CommonLabel, strings.Join(strings.Split(deleteKeys[:], "/")[2], ",")),
				IsMapped:       true,
				MappedResource: newDeploymentMappedResource,
			}, nil
		}
	case *core_v1.Pod:
		var pod core_v1.Pod

		if obj.Event != nil {
			pod = *obj.Event.(*core_v1.Pod).DeepCopy()
		}

		//get replica set from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with RS then it must be created via deployment => RS => pod (Pod name derived from RS)
			//Else Pod is created independently
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "replicaset" && strings.HasPrefix(obj.Name, strings.Split(key, "/")[2]) {
				rsKeys = append(rsKeys, key)
			}
		}

		for _, rsKey := range rsKeys {
			mappedResource, err := getObjectFromStore(rsKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod matched to any of RS
			var newPodSet []core_v1.Pod
			isUpdated := false
			if len(mappedResource.Kube.ReplicaSets) > 0 {
				for _, replicaSet := range mappedResource.Kube.ReplicaSets {
					for _, podOwnerReference := range pod.OwnerReferences {
						if podOwnerReference.Name == replicaSet.Name {
							//pod is matched with this RS. Add pod to this mapped resource
							for _, mappedPod := range mappedResource.Kube.Pods {
								if pod.UID == mappedPod.UID {
									//pod exists. Must have been updated
									newPodSet = append(newPodSet, pod)
									isUpdated = true
								} else {
									newPodSet = append(newPodSet, mappedPod)
								}
							}

							if isUpdated {
								mappedResource.Kube.Pods = nil
								mappedResource.Kube.Pods = newPodSet

								return MapResult{
									Action:         "Updated",
									Key:            rsKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								}, nil
							}

							//If its new pod, add it.
							mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)

							return MapResult{
								Action:         "Added",
								Key:            rsKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}
					}
				}
			}
		}
	case *ext_v1beta1.Ingress:
	default:
		return MapResult{}, fmt.Errorf("Object %s to be mapped from namespace %s of type %s is not supported", obj.Name, obj.Namespace, obj.ResourceType)
	}

	return MapResult{
		IsMapped: false,
	}, nil
}

//Deployment Matching
func deploymentMatching(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var deploymentKeys []string

	//Find matching rs for object to be matched
	switch obj.Event.(type) {
	case *ext_v1beta1.Ingress:
	case *core_v1.Service:
		//Find any deployment matching this service
		var service core_v1.Service

		//get replica set from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with deployment then it must be created via deployment => RS => pod (Pod name derived from deployment)
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "deployment" {
				deploymentKeys = append(deploymentKeys, key)
			}
		}

		if obj.Event != nil {
			service = *obj.Event.(*core_v1.Service).DeepCopy()
		}

		var deleteKeys []string
		newServiceMappedResource := MappedResource{}
		newServiceMappedResource.CommonLabel = service.Name
		newServiceMappedResource.CurrentType = "service"
		newServiceMappedResource.Namespace = service.Namespace
		newServiceMappedResource.Kube.Services = append(newServiceMappedResource.Kube.Services, service)

		for _, deploymentKey := range deploymentKeys {
			mappedResource, err := getObjectFromStore(deploymentKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if deployment is matching to any service
			if len(mappedResource.Kube.Deployments) > 0 {
				for _, deployment := range mappedResource.Kube.Deployments {
					if reflect.DeepEqual(deployment.Spec.Selector.MatchLabels, service.Spec.Selector) {
						//Found match
						//deployment is matched with this service. Add service to mappedResource
						newServiceMappedResource.Kube.Deployments = append(newServiceMappedResource.Kube.Deployments, deployment)
						deleteKeys = append(deleteKeys, deploymentKey)
					}
				}
			}
		}

		if len(deleteKeys) > 0 {
			return MapResult{
				Action:         "Added",
				DeleteKeys:     deleteKeys,
				IsMapped:       true,
				MappedResource: newServiceMappedResource,
			}, nil
		}
	case *apps_v1beta2.Deployment:
		//Update Scenario
		var deployment apps_v1beta2.Deployment

		//get deployment from store
		keys := store.ListKeys()
		for _, key := range keys {
			//Get all resources of types  'deployment' with same name
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "deployment" {
				deploymentKeys = append(deploymentKeys, key)
			}
		}

		if obj.Event != nil {
			deployment = *obj.Event.(*apps_v1beta2.Deployment).DeepCopy()
		}

		for _, deploymentKey := range deploymentKeys {
			mappedResource, err := getObjectFromStore(deploymentKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//Update deployment
			var newDeploymentSet []apps_v1beta2.Deployment
			if len(mappedResource.Kube.Deployments) > 0 {
				for _, mappedDeployment := range mappedResource.Kube.Deployments {
					if deployment.Name == mappedDeployment.Name && deployment.UID == mappedDeployment.UID {
						newDeploymentSet = append(newDeploymentSet, deployment)
					} else {
						newDeploymentSet = append(newDeploymentSet, mappedDeployment)
					}
				}
				mappedResource.Kube.Deployments = nil
				mappedResource.Kube.Deployments = newDeploymentSet

				return MapResult{
					Action:         "Updated",
					Message:        fmt.Sprintf("Deployment '%s' is updated.", deployment.Name),
					Key:            deploymentKey,
					IsMapped:       true,
					MappedResource: mappedResource,
				}, nil
			}
		}
	case *ext_v1beta1.ReplicaSet:
		var replicaSet ext_v1beta1.ReplicaSet

		//get replica sets from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If rs has to map with deployment then it must be created via deployment => RS (RS name derived from deployment)
			//Else RS is created independently
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "deployment" && strings.HasPrefix(obj.Name, strings.Split(key, "/")[2]) {
				deploymentKeys = append(deploymentKeys, key)
			}
		}

		if obj.Event != nil {
			replicaSet = *obj.Event.(*ext_v1beta1.ReplicaSet).DeepCopy()
		}
		// if obj.UpdatedRawObj != nil {
		// 	updatedReplicaSet = *obj.UpdatedRawObj.(*ext_v1beta1.ReplicaSet).DeepCopy()
		// }

		for _, deploymentKey := range deploymentKeys {
			mappedResource, err := getObjectFromStore(deploymentKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod matched to any of RS
			var newRsSet []ext_v1beta1.ReplicaSet
			isUpdated := false
			if len(mappedResource.Kube.Deployments) > 0 {
				for _, deployment := range mappedResource.Kube.Deployments {
					for _, replicaSetOwnerReference := range replicaSet.OwnerReferences {
						if replicaSetOwnerReference.Name == deployment.Name {
							//rs is matched with this Deployment. Add rs to this mapped resource
							for _, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
								if replicaSet.UID == mappedReplicaSet.UID {
									//RS exists. Must have been updated
									newRsSet = append(newRsSet, replicaSet)
									isUpdated = true
								} else {
									newRsSet = append(newRsSet, mappedReplicaSet)
								}
							}

							if isUpdated {
								mappedResource.Kube.ReplicaSets = nil
								mappedResource.Kube.ReplicaSets = newRsSet

								return MapResult{
									Action:         "Updated",
									Message:        fmt.Sprintf("Deployment with common label '%s' is updated with Replica set '%s'", mappedResource.CommonLabel, replicaSet.Name),
									Key:            deploymentKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								}, nil
							}

							//If its new RS, add it.
							mappedResource.Kube.ReplicaSets = append(mappedResource.Kube.ReplicaSets, replicaSet)

							return MapResult{
								Action:         "Updated",
								Message:        fmt.Sprintf("Deployment with common label '%s' is updated with Replica Set '%s'", mappedResource.CommonLabel, replicaSet.Name),
								Key:            deploymentKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}
					}
				}
			}
		}
	case *core_v1.Pod:
		var pod core_v1.Pod

		//get replica set from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with deployment then it may or may not have same name as deployment.
			//Get all 'deployment' type mapped resources and try to map
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "deployment" && strings.HasPrefix(obj.Name, strings.Split(key, "/")[2]) {
				deploymentKeys = append(deploymentKeys, key)
			}
		}

		if obj.Event != nil {
			pod = *obj.Event.(*core_v1.Pod).DeepCopy()
		}

		for _, deploymentKey := range deploymentKeys {
			mappedResource, err := getObjectFromStore(deploymentKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod matched to any of RS in this mapped resources
			var newPodSet []core_v1.Pod
			isUpdated := false
			if len(mappedResource.Kube.ReplicaSets) > 0 {
				for _, replicaSet := range mappedResource.Kube.ReplicaSets {
					var newPodSet []core_v1.Pod
					for _, podOwnerReference := range pod.OwnerReferences {
						if podOwnerReference.Name == replicaSet.Name {
							//pod is matched with this RS. Add pod to this mapped resource
							for _, mappedPod := range mappedResource.Kube.Pods {
								if pod.UID != mappedPod.UID {
									//Not matching pod. Keep them as is in set
									newPodSet = append(newPodSet, mappedPod)
								} else {
									newPodSet = append(newPodSet, pod)
									isUpdated = true
								}
							}

							if isUpdated {
								mappedResource.Kube.Pods = nil
								mappedResource.Kube.Pods = newPodSet

								return MapResult{
									Action:         "Updated",
									Key:            deploymentKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								}, nil
							}

							//If its new pod, add it.
							mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)

							return MapResult{
								Action:         "Added",
								Key:            deploymentKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}
					}
				}
			}

			//Match for deployment
			newPodSet = nil
			isUpdated = false
			if len(mappedResource.Kube.Deployments) > 0 {
				for _, deployment := range mappedResource.Kube.Deployments {
					podMatchedLabels := make(map[string]string)
					for depKey, depValue := range deployment.Spec.Selector.MatchLabels {
						if val, ok := pod.Labels[depKey]; ok {
							if val == depValue {
								podMatchedLabels[depKey] = depValue
							}
						}
					}
					if reflect.DeepEqual(podMatchedLabels, deployment.Spec.Selector.MatchLabels) {
						for _, mappedPod := range mappedResource.Kube.Pods {
							if pod.UID == mappedPod.UID {
								//pod exists. Must have been updated
								newPodSet = append(newPodSet, pod)
								isUpdated = true
							} else {
								newPodSet = append(newPodSet, mappedPod)
							}
						}

						if isUpdated {
							mappedResource.Kube.Pods = nil
							mappedResource.Kube.Pods = newPodSet

							return MapResult{
								Action:         "Updated",
								Key:            deploymentKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}

						//If its new pod, add it.
						mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)

						return MapResult{
							Action:         "Added",
							Key:            deploymentKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}
			}
		}
	default:
		return MapResult{}, fmt.Errorf("Object %s to be mapped from namespace %s of type %s is not supported", obj.Name, obj.Namespace, obj.ResourceType)
	}

	return MapResult{
		IsMapped: false,
	}, nil
}

//Service Matching
func serviceMatching(obj ResourceEvent, store cache.Store, serviceName ...string) ([]MapResult, error) {
	var serviceKeys []string
	var mapResults []MapResult

	//Find matching rs for object to be matched
	switch obj.Event.(type) {
	case *ext_v1beta1.Ingress:
		var ingress ext_v1beta1.Ingress

		if obj.Event != nil {
			ingress = *obj.Event.(*ext_v1beta1.Ingress).DeepCopy()
		}

		if len(serviceName) > 0 {
			var ingressKeys []string
			keys := store.ListKeys()
			for _, key := range keys {
				//If pod has to map with deployment then it may or may not have same name as deployment.
				//Get all 'deployment' type mapped resources and try to map
				if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
					ingressKeys = append(ingressKeys, key)
				}
			}

			var newIngressSet []ext_v1beta1.Ingress
			for _, ingressKey := range ingressKeys {
				newIngressSet = nil

				mappedResource, err := getObjectFromStore(ingressKey, store)
				if err != nil {
					//Override error to avoid return.
					//Set it as not mapped so ingress will get created as individual resource.
					return []MapResult{
						MapResult{
							IsMapped: false,
						},
					}, nil
				}

				if len(mappedResource.Kube.Services) > 0 {
					for _, mappedService := range mappedResource.Kube.Services {
						isUpdated := false
						if mappedService.Name == serviceName[0] {
							//Service already exists. Add ingress to it.
							for _, mappedIngress := range mappedResource.Kube.Ingresses {
								if mappedIngress.UID == ingress.UID {
									newIngressSet = append(newIngressSet, ingress)
									isUpdated = true
								} else {
									newIngressSet = append(newIngressSet, mappedIngress)
								}
							}

							if isUpdated {
								mappedResource.Kube.Ingresses = nil
								mappedResource.Kube.Ingresses = newIngressSet

								// //Update Common Label
								// if len(mappedResource.Kube.Ingresses) > 0 {
								// 	var ingressNames []string
								// 	for _, mappedIngress := range mappedResource.Kube.Ingresses {
								// 		ingressNames = append(ingressNames, mappedIngress.Name)
								// 	}
								// 	mappedResource.CommonLabel = strings.Join(ingressNames, ", ")
								// }

								result := MapResult{
									Action:         "Updated",
									Key:            ingressKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								}
								mapResults = append(mapResults, result)
								return mapResults, nil
							}

							mappedResource.Kube.Ingresses = append(mappedResource.Kube.Ingresses, ingress)

							// //Update Common Label
							// if len(mappedResource.Kube.Ingresses) > 0 {
							// 	// for _, mappedIngress := range mappedResource.Kube.Ingresses {
							// 	// 	mappedResource.CommonLabel = mappedIngress.Name
							// 	// 	break
							// 	// }

							// 	var ingressNames []string
							// 	for _, mappedIngress := range mappedResource.Kube.Ingresses {
							// 		ingressNames = append(ingressNames, mappedIngress.Name)
							// 	}
							// 	commonLabel := strings.Join(ingressNames, "-")
							// 	if commonLabel != mappedResource.CommonLabel {
							// 		mappedResource.CommonLabel = strings.Join(ingressNames, "-")

							// 		//Since we updated the common label. Delete old resource from local store
							// 		deleteResource := MapResult{
							// 			Action:         "ForceDeleted",
							// 			Key:            ingressKey,
							// 			IsMapped:       true,
							// 			IsStoreUpdated: true, //Make it as store updated so it will be just sent as an event
							// 			MappedResource: copyOfMappedResource,
							// 			CommonLabel:    copyOfMappedResource.CommonLabel,
							// 		}

							// 		mapResults = append(mapResults, deleteResource)
							// 	}
							// }

							result := MapResult{
								Action:         "Updated",
								Key:            ingressKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}

							mapResults = append(mapResults, result)

							return mapResults, nil
						}
					}
				}
			}
		}

	case *core_v1.Service:
		//This handles update of service. If service exists, it must have been updated.
		//If its not an update, check if the we have any ingress mapped resource, create as type 'service'
		var service core_v1.Service

		//get service from store
		keys := store.ListKeys()
		for _, key := range keys {
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
				serviceKeys = append(serviceKeys, key)
			}
		}

		if obj.Event != nil {
			service = *obj.Event.(*core_v1.Service).DeepCopy()
		}

		switch obj.EventType {
		case "UPDATED":
			for _, serviceKey := range serviceKeys {
				mappedResource, err := getObjectFromStore(serviceKey, store)
				if err != nil {
					return []MapResult{}, err
				}

				//Check if we have to update service
				var newServiceSet []core_v1.Service
				//isUpdated := false
				if len(mappedResource.Kube.Services) > 0 {
					for _, mappedService := range mappedResource.Kube.Services {
						if mappedService.Name != service.Name {
							newServiceSet = append(newServiceSet, mappedService)
						} else {
							newServiceSet = append(newServiceSet, service)
							//isUpdated = true
						}
					}

					//if isUpdated {
					mappedResource.Kube.Services = nil
					mappedResource.Kube.Services = newServiceSet

					return []MapResult{
						MapResult{
							Action:         "Updated",
							Key:            serviceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						},
					}, nil
					//}
				}

				//Check if its actually an ingress created as type 'service'
				if len(mappedResource.Kube.Ingresses) > 0 {
					for _, mappedIngress := range mappedResource.Kube.Ingresses {
						for _, ingressRule := range mappedIngress.Spec.Rules {
							if ingressRule.IngressRuleValue.HTTP != nil {
								for _, ingressRuleValueHTTPPath := range ingressRule.IngressRuleValue.HTTP.Paths {
									if ingressRuleValueHTTPPath.Backend.ServiceName != "" {
										if ingressRuleValueHTTPPath.Backend.ServiceName == service.Name {
											//Add this service to ingress created as type 'service'
											//Common Label is already set to ingress name.
											mappedResource.Kube.Services = append(mappedResource.Kube.Services, service)
											mappedResource.CommonLabel = service.Name

											return []MapResult{
												MapResult{
													Action:         "Updated",
													Key:            serviceKey,
													IsMapped:       true,
													MappedResource: mappedResource,
												},
											}, nil
										}
									}
								}
							}
						}
					}
				}
			}
		}
	case *apps_v1beta2.Deployment:
		var deployment apps_v1beta2.Deployment
		serviceKeys = nil

		//get service from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If deployment has to map with service then they must have same match labels.
			//Get all 'service' type mapped resources and try to map
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
				serviceKeys = append(serviceKeys, key)
			}
		}

		if obj.Event != nil {
			deployment = *obj.Event.(*apps_v1beta2.Deployment).DeepCopy()
		}

		var newDeploymentSet []apps_v1beta2.Deployment
		for _, serviceKey := range serviceKeys {
			newDeploymentSet = nil
			mappedResource, err := getObjectFromStore(serviceKey, store)
			if err != nil {
				return []MapResult{}, err
			}

			//See if deployment match to any of service
			if len(mappedResource.Kube.Services) > 0 {
				for _, service := range mappedResource.Kube.Services {
					if reflect.DeepEqual(deployment.Spec.Selector.MatchLabels, service.Spec.Selector) {
						//deployment is matched with this service. Add deployment to this mapped resource
						isUpdated := false
						for _, mappedDeployment := range mappedResource.Kube.Deployments {
							if deployment.UID == mappedDeployment.UID {
								//Deployment exists. Must have been updated
								newDeploymentSet = append(newDeploymentSet, deployment)
								isUpdated = true
							} else {
								newDeploymentSet = append(newDeploymentSet, mappedDeployment)
							}
						}

						if isUpdated {
							mappedResource.Kube.Deployments = nil
							mappedResource.Kube.Deployments = newDeploymentSet
							return []MapResult{
								MapResult{
									Action:         "Updated",
									Key:            serviceKey,
									Message:        fmt.Sprintf("Deployment '%s' is updated in common label '%s'", deployment.Name, mappedResource.CommonLabel),
									IsMapped:       true,
									MappedResource: mappedResource,
								},
							}, nil
						}

						//If its new deployment, add it.
						mappedResource.Kube.Deployments = append(mappedResource.Kube.Deployments, deployment)

						return []MapResult{
							MapResult{
								Action:         "Added",
								Key:            serviceKey,
								Message:        fmt.Sprintf("Deployment '%s' is added in common label '%s'", deployment.Name, mappedResource.CommonLabel),
								IsMapped:       true,
								MappedResource: mappedResource,
							},
						}, nil
					}
				}
			} else if len(mappedResource.Kube.Deployments) > 0 {
				isUpdated := false
				for _, mappedDeployment := range mappedResource.Kube.Deployments {
					if deployment.UID == mappedDeployment.UID {
						//Deployment exists. Must have been updated
						newDeploymentSet = append(newDeploymentSet, deployment)
						isUpdated = true
					} else {
						newDeploymentSet = append(newDeploymentSet, mappedDeployment)
					}
				}

				if isUpdated {
					mappedResource.Kube.Deployments = nil
					mappedResource.Kube.Deployments = newDeploymentSet
					return []MapResult{
						MapResult{
							Action:         "Updated",
							Key:            serviceKey,
							Message:        fmt.Sprintf("Deployment '%s' is updated in common label '%s'", deployment.Name, mappedResource.CommonLabel),
							IsMapped:       true,
							MappedResource: mappedResource,
						},
					}, nil
				}

				//If its new deployment, add it.
				mappedResource.Kube.Deployments = append(mappedResource.Kube.Deployments, deployment)

				return []MapResult{
					MapResult{
						Action:         "Added",
						Key:            serviceKey,
						Message:        fmt.Sprintf("Deployment '%s' is added in common label '%s'", deployment.Name, mappedResource.CommonLabel),
						IsMapped:       true,
						MappedResource: mappedResource,
					},
				}, nil
			}
		}

	case *ext_v1beta1.ReplicaSet:
		var replicaSet ext_v1beta1.ReplicaSet

		//get service from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with service then it may or may not have same name as service.
			//Get all 'service' type mapped resources and try to map
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
				serviceKeys = append(serviceKeys, key)
			}
		}

		if obj.Event != nil {
			replicaSet = *obj.Event.(*ext_v1beta1.ReplicaSet).DeepCopy()
		}

		for _, serviceKey := range serviceKeys {
			mappedResource, err := getObjectFromStore(serviceKey, store)
			if err != nil {
				return []MapResult{}, err
			}

			//Check Deployment followed by service
			//See if RS matching to any of Deployment
			var newRsSet []ext_v1beta1.ReplicaSet
			isUpdated := false
			if len(mappedResource.Kube.Deployments) > 0 {
				for _, deployment := range mappedResource.Kube.Deployments {
					for _, replicaSetOwnerReference := range replicaSet.OwnerReferences {
						if replicaSetOwnerReference.Name == deployment.Name {
							//rs is matched with this Deployment. Add rs to this mapped resource
							for _, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
								if replicaSet.UID == mappedReplicaSet.UID {
									//RS exists. Must have been updated
									newRsSet = append(newRsSet, replicaSet)
									isUpdated = true
								} else {
									newRsSet = append(newRsSet, mappedReplicaSet)
								}
							}

							if isUpdated {
								mappedResource.Kube.ReplicaSets = nil
								mappedResource.Kube.ReplicaSets = newRsSet

								return []MapResult{
									MapResult{
										Action:         "Updated",
										Message:        fmt.Sprintf("Service with common label '%s' is updated with Replica set '%s'", mappedResource.CommonLabel, replicaSet.Name),
										Key:            serviceKey,
										IsMapped:       true,
										MappedResource: mappedResource,
									},
								}, nil
							}

							//If its new RS, add it.
							mappedResource.Kube.ReplicaSets = append(mappedResource.Kube.ReplicaSets, replicaSet)

							return []MapResult{
								MapResult{
									Action:         "Updated",
									Message:        fmt.Sprintf("Service with common label '%s' is updated with Replica sets '%s'", mappedResource.CommonLabel, replicaSet.Name),
									Key:            serviceKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								},
							}, nil
						}
					}
				}
			}

			//Match for service
			newRsSet = nil
			isUpdated = false
			if len(mappedResource.Kube.Services) > 0 {
				for _, service := range mappedResource.Kube.Services {
					rsMatchedLabels := make(map[string]string)
					for svcKey, svcValue := range service.Spec.Selector {
						if val, ok := replicaSet.Spec.Selector.MatchLabels[svcKey]; ok {
							if val == svcValue {
								rsMatchedLabels[svcKey] = svcValue
							}
						}
					}
					if reflect.DeepEqual(rsMatchedLabels, service.Spec.Selector) {
						//rs is matched with this Deployment. Add rs to this mapped resource
						for _, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
							if replicaSet.UID == mappedReplicaSet.UID {
								//RS exists. Must have been updated
								newRsSet = append(newRsSet, replicaSet)
								isUpdated = true
							} else {
								newRsSet = append(newRsSet, mappedReplicaSet)
							}
						}

						if isUpdated {
							mappedResource.Kube.ReplicaSets = nil
							mappedResource.Kube.ReplicaSets = newRsSet

							return []MapResult{
								MapResult{
									Action:         "Updated",
									Message:        fmt.Sprintf("Service with common label '%s' is updated with Replica sets '%s'", mappedResource.CommonLabel, replicaSet.Name),
									Key:            serviceKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								},
							}, nil
						}

						//If its new RS, add it.
						mappedResource.Kube.ReplicaSets = append(mappedResource.Kube.ReplicaSets, replicaSet)

						return []MapResult{
							MapResult{
								Action:         "Updated",
								Message:        fmt.Sprintf("Service with common label '%s' is updated with Replica sets '%s'", mappedResource.CommonLabel, replicaSet.Name),
								Key:            serviceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							},
						}, nil
					}
				}
			}
		}

	case *core_v1.Pod:
		var pod core_v1.Pod
		serviceKeys = nil

		//get service from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with service then it may or may not have same name as service.
			//Get all 'service' type mapped resources and try to map
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
				serviceKeys = append(serviceKeys, key)
			}
		}

		if obj.Event != nil {
			pod = *obj.Event.(*core_v1.Pod).DeepCopy()
		}

		for _, serviceKey := range serviceKeys {
			mappedResource, err := getObjectFromStore(serviceKey, store)
			if err != nil {
				return []MapResult{}, err
			}

			var newPodSet []core_v1.Pod
			//Match for service
			if len(mappedResource.Kube.Services) > 0 {
				for _, service := range mappedResource.Kube.Services {
					podMatchedLabels := make(map[string]string)
					for svcKey, svcValue := range service.Spec.Selector {
						if val, ok := pod.Labels[svcKey]; ok {
							if val == svcValue {
								podMatchedLabels[svcKey] = svcValue
							}
						}
					}
					if reflect.DeepEqual(podMatchedLabels, service.Spec.Selector) {
						isUpdated := false
						for _, mappedPod := range mappedResource.Kube.Pods {
							if pod.UID == mappedPod.UID {
								//pod exists. Must have been updated
								newPodSet = append(newPodSet, pod)
								isUpdated = true
							} else {
								newPodSet = append(newPodSet, mappedPod)
							}
						}

						if isUpdated {
							mappedResource.Kube.Pods = nil
							mappedResource.Kube.Pods = newPodSet

							return []MapResult{
								MapResult{
									Action:         "Updated",
									Key:            serviceKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								},
							}, nil
						}

						//If its new pod, add it.
						mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)

						return []MapResult{
							MapResult{
								Action:         "Added",
								Key:            serviceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							},
						}, nil
					}
				}
			} else if len(mappedResource.Kube.ReplicaSets) > 0 { //See if pod is matching to any of RS in this mapped resources
				isUpdated := false
				for _, replicaSet := range mappedResource.Kube.ReplicaSets {
					for _, podOwnerReference := range pod.OwnerReferences {
						if podOwnerReference.Name == replicaSet.Name {
							//pod is matched with this RS. Add pod to this mapped resource
							for _, mappedPod := range mappedResource.Kube.Pods {
								if pod.UID == mappedPod.UID {
									//pod exists. Must have been updated
									newPodSet = append(newPodSet, pod)
									isUpdated = true
								} else {
									newPodSet = append(newPodSet, mappedPod)
								}
							}

							if isUpdated {
								mappedResource.Kube.Pods = nil
								mappedResource.Kube.Pods = newPodSet

								return []MapResult{
									MapResult{
										Action:         "Updated",
										Key:            serviceKey,
										IsMapped:       true,
										MappedResource: mappedResource,
									},
								}, nil
							}

							//If its new pod, add it.
							mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)

							return []MapResult{
								MapResult{
									Action:         "Added",
									Key:            serviceKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								},
							}, nil
						}
					}
				}
			} else if len(mappedResource.Kube.Deployments) > 0 { //Match for deployment
				for _, deployment := range mappedResource.Kube.Deployments {
					podMatchedLabels := make(map[string]string)
					for depKey, depValue := range deployment.Spec.Selector.MatchLabels {
						if val, ok := pod.Labels[depKey]; ok {
							if val == depValue {
								podMatchedLabels[depKey] = depValue
							}
						}
					}
					if reflect.DeepEqual(podMatchedLabels, deployment.Spec.Selector.MatchLabels) {
						isUpdated := false
						for _, mappedPod := range mappedResource.Kube.Pods {
							if pod.UID == mappedPod.UID {
								//pod exists. Must have been updated
								newPodSet = append(newPodSet, pod)
								isUpdated = true
							} else {
								newPodSet = append(newPodSet, mappedPod)
							}
						}

						if isUpdated {
							mappedResource.Kube.Pods = nil
							mappedResource.Kube.Pods = newPodSet

							return []MapResult{
								MapResult{
									Action:         "Updated",
									Key:            serviceKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								},
							}, nil
						}

						//If its new pod, add it.
						mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)

						return []MapResult{
							MapResult{
								Action:         "Added",
								Key:            serviceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							},
						}, nil
					}
				}
			} else if len(mappedResource.Kube.Pods) > 0 { //Match for pods
				isUpdated := false
				for _, mappedPod := range mappedResource.Kube.Pods {
					if mappedPod.Name == pod.Name && mappedPod.UID == pod.UID {
						//Update it
						newPodSet = append(newPodSet, pod)
						isUpdated = true
					} else {
						newPodSet = append(newPodSet, mappedPod)
					}
				}

				if isUpdated {
					mappedResource.Kube.Pods = nil
					mappedResource.Kube.Pods = newPodSet

					return []MapResult{
						MapResult{
							Action:         "Added",
							Key:            serviceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						},
					}, nil
				}
			}
		}
	default:
		return []MapResult{}, fmt.Errorf("Object to be mapped is not supported")
	}

	return []MapResult{
		MapResult{
			IsMapped: false,
		},
	}, nil
}

func getObjectFromStore(key string, store cache.Store) (MappedResource, error) {
	item, exists, err := store.GetByKey(key)

	if err != nil {
		return MappedResource{}, err
	}

	if exists {
		return item.(MappedResource), nil
	}
	return MappedResource{}, fmt.Errorf("Object with key %s does not exist in store", key)
}

func updateStore(results []MapResult, store cache.Store) error {
	for _, result := range results {
		if result.IsMapped && !result.IsStoreUpdated {
			switch result.Action {
			case "Added", "Updated":
				if result.Key != "" {
					//Update object in store
					existingMappedResource, err := getObjectFromStore(result.Key, store)
					if err != nil {
						return err
					}

					//Delete exiting resource from store
					err = store.Delete(existingMappedResource)
					if err != nil {
						return err
					}

					//Add new mapped resource to store
					err = store.Add(result.MappedResource)
					if err != nil {
						return err
					}
				} else if len(result.DeleteKeys) > 0 {
					//Needs to delete multiple resources
					//Update object in store
					for _, deleteKey := range result.DeleteKeys {
						existingMappedResource, err := getObjectFromStore(deleteKey, store)
						if err != nil {
							return err
						}

						//Delete exiting resource from store
						err = store.Delete(existingMappedResource)
						if err != nil {
							return err
						}
					}

					//Add new mapped resource to store
					err := store.Add(result.MappedResource)
					if err != nil {
						return err
					}
				} else {
					//If key is not present then its new mapped resource.
					//Add new individual mapped resource to store
					err := store.Add(result.MappedResource)
					if err != nil {
						return err
					}
				}
			case "Deleted":
				if result.Key != "" {
					//Get object from store
					existingMappedResource, err := getObjectFromStore(result.Key, store)
					if err != nil {
						return err
					}

					//Delete existing resource from store
					err = store.Delete(existingMappedResource)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
