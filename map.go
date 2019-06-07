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

		return []MapResult{
			mappedService,
		}, nil
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
	var serviceMappingResult MapResult
	var err error
	var ingressBackendServices []string
	var mapResults []MapResult

	if obj.RawObj != nil {
		ingress = *obj.RawObj.(*ext_v1beta1.Ingress).DeepCopy()
	}

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
		serviceMappingResult, err = serviceMatching(obj, store, ingressBackendService)
		if err != nil {
			return []MapResult{}, err
		}

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

	return mapResults, nil
}

/*
	Service can either be independent resource or mapped to ingress
	Service can also be mapped to individual existing deployment/s, replica set/s or pod/s from store.
*/
func mapService(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var service core_v1.Service
	var serviceMappingResult, podMappingResult, replicaSetMappingResult, deploymentMappingResult MapResult
	var err error

	//Try matching with service
	serviceMappingResult, err = serviceMatching(obj, store)
	if err != nil {
		return MapResult{}, err
	}

	if !serviceMappingResult.IsMapped {
		deploymentMappingResult, err = deploymentMatching(obj, store)
		if err != nil {
			return MapResult{}, err
		}

		if !deploymentMappingResult.IsMapped {
			//Try matching with replica set
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
					//Service not mapped with any existing mapped resources. Create as an individual resource
					if obj.RawObj != nil {
						service = *obj.RawObj.(*core_v1.Service).DeepCopy()
					}

					mappedIndividualResource := MappedResource{}
					mappedIndividualResource.CommonLabel = service.Name
					mappedIndividualResource.CurrentType = "service"
					mappedIndividualResource.Namespace = service.Namespace
					mappedIndividualResource.Kube.Services = append(mappedIndividualResource.Kube.Services, service)

					return MapResult{
						Action:         "Added",
						IsMapped:       true,
						MappedResource: mappedIndividualResource,
					}, nil
				}

				return podMappingResult, nil
			}

			return replicaSetMappingResult, nil
		}

		return deploymentMappingResult, nil
	}

	return serviceMappingResult, nil
}

/*
	Deployment can either be independent resource or mapped to service
	Deployment can also be mapped to individual existing pod/s or replica set/s, from store.
*/
func mapDeployment(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var deployment apps_v1beta2.Deployment
	var podMappingResult, replicaSetMappingResult, serviceMappingResult MapResult
	var err error

	//Try matching with service
	serviceMappingResult, err = serviceMatching(obj, store)
	if err != nil {
		return MapResult{}, err
	}

	if !serviceMappingResult.IsMapped {
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
				if obj.RawObj != nil {
					deployment = *obj.RawObj.(*apps_v1beta2.Deployment).DeepCopy()
				}

				mappedIndividualResource := MappedResource{}
				mappedIndividualResource.CommonLabel = deployment.Name
				mappedIndividualResource.CurrentType = "deployment"
				mappedIndividualResource.Namespace = deployment.Namespace
				mappedIndividualResource.Kube.Deployments = append(mappedIndividualResource.Kube.Deployments, deployment)

				return MapResult{
					Action:         "Added",
					IsMapped:       true,
					MappedResource: mappedIndividualResource,
				}, nil
			}

			return podMappingResult, nil
		}

		return replicaSetMappingResult, nil
	}

	return serviceMappingResult, nil
}

/*
	Replica Set can either be independent resource or mapped to deployment or service
	RS can also be mapped to individual existing pod from store.
*/
func mapReplicaSet(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var replicaSet ext_v1beta1.ReplicaSet
	var podMappingResult, deploymentMappingResult, serviceMappingResult MapResult
	var err error

	//Try matching with deployment
	deploymentMappingResult, err = deploymentMatching(obj, store)
	if err != nil {
		return MapResult{}, err
	}

	//Try matching with service
	if !deploymentMappingResult.IsMapped {
		serviceMappingResult, err = serviceMatching(obj, store)
		if err != nil {
			return MapResult{}, err
		}

		//Try matching with any individual pod
		if !serviceMappingResult.IsMapped {
			podMappingResult, err = podMatching(obj, store)
			if err != nil {
				return MapResult{}, err
			}

			if !podMappingResult.IsMapped {
				//RS not mapped with any existing mapped resources. Create as an individual resource
				if obj.RawObj != nil {
					replicaSet = *obj.RawObj.(*ext_v1beta1.ReplicaSet).DeepCopy()
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
		}
		return serviceMappingResult, nil

	}

	return deploymentMappingResult, nil
}

/*
	Pod can either be independent resource or mapped to rs, deployment or service
*/
func mapPod(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var pod core_v1.Pod
	var rsMappingResult, deploymentMappingResult, serviceMappingResult MapResult
	var err error

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
			serviceMappingResult, err = serviceMatching(obj, store)
			if err != nil {
				return MapResult{}, err
			}

			if !serviceMappingResult.IsMapped {
				//It's an individual pod. Create it
				if obj.RawObj != nil {
					pod = *obj.RawObj.(*core_v1.Pod).DeepCopy()
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

			return serviceMappingResult, nil
		}

		return deploymentMappingResult, nil
	}

	return rsMappingResult, nil
}

// //Ingress Matching
// func ingressMatching(obj ResourceEvent, store cache.Store) (MapResult, error) {
// 	// var ingressKeys []string

// 	// switch obj.RawObj.(type) {
// 	// case *ext_v1beta1.Ingress:
// 	// case *core_v1.Service:
// 	// 	//Find any individual ingress matching this service
// 	// 	var service core_v1.Service

// 	// 	//get ingress store
// 	// 	keys := store.ListKeys()
// 	// 	for _, key := range keys {
// 	// 		//If pod has to map with deployment then it must be created via deployment => RS => pod (Pod name derived from deployment)
// 	// 		if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "ingress" {
// 	// 			ingressKeys = append(ingressKeys, key)
// 	// 		}
// 	// 	}

// 	// 	if obj.RawObj != nil {
// 	// 		service = *obj.RawObj.(*core_v1.Service).DeepCopy()
// 	// 	}

// 	// 	for _, ingressKey := range ingressKeys {
// 	// 		mappedResource, err := getObjectFromStore(ingressKey, store)
// 	// 		if err != nil {
// 	// 			return MapResult{}, err
// 	// 		}

// 	// 		//See if ingress is matching to any Service
// 	// 		if len(mappedResource.Kube.Ingresses) > 0 {
// 	// 			for _, mappedIngress := range mappedResource.Kube.Ingresses {
// 	// 				for _, ingressRule := range mappedIngress.Spec.Rules {
// 	// 					if ingressRule.IngressRuleValue.HTTP != nil {
// 	// 						for _, ingressRuleValueHTTPPath := range ingressRule.IngressRuleValue.HTTP.Paths {
// 	// 							if ingressRuleValueHTTPPath.Backend.ServiceName != "" {
// 	// 								if ingressRuleValueHTTPPath.Backend.ServiceName == service.Name {
// 	// 									//Add this ingress to service
// 	// 								}
// 	// 							}
// 	// 						}
// 	// 					}
// 	// 				}
// 	// 			}
// 	// 		}
// 	// 	}
// 	// }
// }

//Pod Matching
func podMatching(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var podKeys []string

	//Find matching rs for object to be matched
	switch obj.RawObj.(type) {
	case *ext_v1beta1.Ingress:
	case *core_v1.Service:
		//Find any individual pods matching this service
		var service core_v1.Service

		//get services from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with deployment then it must be created via deployment => RS => pod (Pod name derived from deployment)
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
				podKeys = append(podKeys, key)
			}
		}

		if obj.RawObj != nil {
			service = *obj.RawObj.(*core_v1.Service).DeepCopy()
		}

		for _, podKey := range podKeys {
			mappedResource, err := getObjectFromStore(podKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod matched to any of RS
			if len(mappedResource.Kube.Pods) > 0 {
				for _, pod := range mappedResource.Kube.Pods {
					isMatching := false
					for svcKey, svcValue := range service.Spec.Selector {
						if val, ok := pod.Labels[svcKey]; ok {
							if val == svcValue {
								isMatching = true
							} else {
								isMatching = false
							}
						}
					}
					if isMatching {
						//Add service to this RS mapped resources
						mappedResource.Kube.Services = append(mappedResource.Kube.Services, service)
						mappedResource.CommonLabel = service.Name
						mappedResource.CurrentType = "service"

						return MapResult{
							Action:         "Added",
							Key:            podKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}
			}
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

		if obj.RawObj != nil {
			deployment = *obj.RawObj.(*apps_v1beta2.Deployment).DeepCopy()
		}

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
						mappedResource.Kube.Deployments = append(mappedResource.Kube.Deployments, deployment)
						mappedResource.CommonLabel = deployment.Name
						mappedResource.CurrentType = "deployment"
						return MapResult{
							Action:         "Added",
							Key:            podKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}
			}
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

		if obj.RawObj != nil {
			replicaSet = *obj.RawObj.(*ext_v1beta1.ReplicaSet).DeepCopy()
		}

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
							//pod is matched with this RS. Add RS to updatedMappedResource
							mappedResource.Kube.ReplicaSets = append(mappedResource.Kube.ReplicaSets, replicaSet)
							mappedResource.CommonLabel = replicaSet.Name
							mappedResource.CurrentType = "replicaset"
							return MapResult{
								Action:         "Added",
								Key:            podKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}
					}
				}
			}
		}

	case *core_v1.Pod:
	default:
		return MapResult{}, fmt.Errorf("Object %s to be mapped from namespace %s is not supported", obj.Name, obj.Namespace)
	}

	return MapResult{
		IsMapped: false,
	}, nil
}

//Replica Set Matching
func replicaSetMatching(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var rsKeys []string

	//Find matching rs for object to be matched
	if len(rsKeys) > 0 {
		switch obj.RawObj.(type) {
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

			if obj.RawObj != nil {
				service = *obj.RawObj.(*core_v1.Service).DeepCopy()
			}

			for _, rsKey := range rsKeys {
				mappedResource, err := getObjectFromStore(rsKey, store)
				if err != nil {
					return MapResult{}, err
				}

				//See if deployment is matching to any service
				if len(mappedResource.Kube.ReplicaSets) > 0 {
					for _, replicaSet := range mappedResource.Kube.ReplicaSets {
						isMatching := false
						for svcKey, svcValue := range service.Spec.Selector {
							if val, ok := replicaSet.Spec.Selector.MatchLabels[svcKey]; ok {
								if val == svcValue {
									isMatching = true
								} else {
									isMatching = false
								}
							}
						}
						if isMatching {
							//Add service to this RS mapped resources
							mappedResource.Kube.Services = append(mappedResource.Kube.Services, service)
							mappedResource.CommonLabel = service.Name
							mappedResource.CurrentType = "service"

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
		case *ext_v1beta1.ReplicaSet:
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

			if obj.RawObj != nil {
				deployment = *obj.RawObj.(*apps_v1beta2.Deployment).DeepCopy()
			}

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
								mappedResource.Kube.Deployments = append(mappedResource.Kube.Deployments, deployment)
								mappedResource.CommonLabel = deployment.Name
								mappedResource.CurrentType = "deployment"
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
		case *core_v1.Pod:
			var pod, updatedPod core_v1.Pod

			if obj.RawObj != nil {
				pod = *obj.RawObj.(*core_v1.Pod).DeepCopy()
			}
			if obj.UpdatedRawObj != nil {
				updatedPod = *obj.UpdatedRawObj.(*core_v1.Pod).DeepCopy()
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
				if len(mappedResource.Kube.ReplicaSets) > 0 {
					for _, replicaSet := range mappedResource.Kube.ReplicaSets {
						for _, podOwnerReference := range pod.OwnerReferences {
							if podOwnerReference.Name == replicaSet.Name {
								//pod is matched with this RS. Add pod to this mapped resource
								for _, mappedPod := range mappedResource.Kube.Pods {
									if pod.UID == mappedPod.UID {
										//pod exists. Must have been updated
										mappedPod = updatedPod
									}

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
			return MapResult{}, fmt.Errorf("Object %s to be mapped from namespace %s is not supported", obj.Name, obj.Namespace)
		}
	}

	return MapResult{
		IsMapped: false,
	}, nil
}

//Deployment Matching
func deploymentMatching(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var deploymentKeys []string

	//Find matching rs for object to be matched
	switch obj.RawObj.(type) {
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

		if obj.RawObj != nil {
			service = *obj.RawObj.(*core_v1.Service).DeepCopy()
		}

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
						mappedResource.Kube.Services = append(mappedResource.Kube.Services, service)
						mappedResource.CommonLabel = service.Name
						mappedResource.CurrentType = "service"
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
	case *apps_v1beta2.Deployment:
	case *ext_v1beta1.ReplicaSet:
		var replicaSet, updatedReplicaSet ext_v1beta1.ReplicaSet

		//get replica sets from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If rs has to map with deployment then it must be created via deployment => RS (RS name derived from deployment)
			//Else Pod is created independently
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "deployment" && strings.HasPrefix(obj.Name, strings.Split(key, "/")[2]) {
				deploymentKeys = append(deploymentKeys, key)
			}
		}

		if obj.RawObj != nil {
			replicaSet = *obj.RawObj.(*ext_v1beta1.ReplicaSet).DeepCopy()
		}
		if obj.UpdatedRawObj != nil {
			updatedReplicaSet = *obj.UpdatedRawObj.(*ext_v1beta1.ReplicaSet).DeepCopy()
		}

		for _, deploymentKey := range deploymentKeys {
			mappedResource, err := getObjectFromStore(deploymentKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod matched to any of RS
			if len(mappedResource.Kube.Deployments) > 0 {
				for _, deployment := range mappedResource.Kube.Deployments {
					for _, replicaSetOwnerReference := range replicaSet.OwnerReferences {
						if replicaSetOwnerReference.Name == deployment.Name {
							//rs is matched with this Deployment. Add rs to this mapped resource
							for _, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
								if replicaSet.UID == mappedReplicaSet.UID {
									//RS exists. Must have been updated
									mappedReplicaSet = updatedReplicaSet
								}

								return MapResult{
									Action:         "Updated",
									Key:            deploymentKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								}, nil
							}

							//If its new pod, add it.
							mappedResource.Kube.ReplicaSets = append(mappedResource.Kube.ReplicaSets, replicaSet)

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
		}
	case *core_v1.Pod:
		//get replica set from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with deployment then it may or may not have same name as deployment.
			//Get all 'deployment' type mapped resources and try to map
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "deployment" {
				deploymentKeys = append(deploymentKeys, key)
			}
		}

		var pod, updatedPod core_v1.Pod

		if obj.RawObj != nil {
			pod = *obj.RawObj.(*core_v1.Pod).DeepCopy()
		}
		if obj.UpdatedRawObj != nil {
			updatedPod = *obj.UpdatedRawObj.(*core_v1.Pod).DeepCopy()
		}

		for _, deploymentKey := range deploymentKeys {
			mappedResource, err := getObjectFromStore(deploymentKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod matched to any of RS in this mapped resources
			if len(mappedResource.Kube.ReplicaSets) > 0 {
				for _, replicaSet := range mappedResource.Kube.ReplicaSets {
					for _, podOwnerReference := range pod.OwnerReferences {
						if podOwnerReference.Name == replicaSet.Name {
							//pod is matched with this RS. Add pod to this mapped resource
							for _, mappedPod := range mappedResource.Kube.Pods {
								if pod.UID == mappedPod.UID {
									//pod exists. Must have been updated
									mappedPod = updatedPod
								}

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
			if len(mappedResource.Kube.Deployments) > 0 {
				for _, deployment := range mappedResource.Kube.Deployments {
					isMatching := false
					for depKey, depValue := range deployment.Spec.Selector.MatchLabels {
						if val, ok := pod.Labels[depKey]; ok {
							if val == depValue {
								isMatching = true
							} else {
								isMatching = false
							}
						}
					}
					if isMatching {
						for _, mappedPod := range mappedResource.Kube.Pods {
							if pod.UID == mappedPod.UID {
								//pod exists. Must have been updated
								mappedPod = updatedPod
							}

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
		return MapResult{}, fmt.Errorf("Object %s to be mapped from namespace %s is not supported", obj.Name, obj.Namespace)
	}

	return MapResult{
		IsMapped: false,
	}, nil
}

//Service Matching
func serviceMatching(obj ResourceEvent, store cache.Store, serviceName ...string) (MapResult, error) {
	var serviceKeys []string

	//Find matching rs for object to be matched
	switch obj.RawObj.(type) {
	case *ext_v1beta1.Ingress:
		var ingress ext_v1beta1.Ingress

		if obj.RawObj != nil {
			ingress = *obj.RawObj.(*ext_v1beta1.Ingress).DeepCopy()
		}

		if len(serviceName) > 0 {
			ingressKey := fmt.Sprintf("%s/service/%s", ingress.Namespace, serviceName[0])

			mappedResource, err := getObjectFromStore(ingressKey, store)
			if err != nil {
				//Override error to avoid return.
				//Set is not map so ingress will get created individual resource.
				return MapResult{
					IsMapped: false,
				}, nil
			}

			if len(mappedResource.Kube.Services) > 0 {
				for _, mappedService := range mappedResource.Kube.Services {
					if mappedService.Name == serviceName[0] {
						//Service already exists. Add ingress to it.
						if len(mappedResource.Kube.Ingresses) == 0 {
							mappedResource.CommonLabel = ingress.Name
						}

						mappedResource.Kube.Ingresses = append(mappedResource.Kube.Ingresses, ingress)

						return MapResult{
							Action:         "Updated",
							Key:            ingressKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}
			}
		}

	case *core_v1.Service:
		//This handles update of service. If service exists, it must have been updated.
		//If its not an update, check if the we have any ingress mapped resource, create as type 'service'
		var service, updatedService core_v1.Service

		//get service from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If deployment has to map with service then they must have same match labels.
			//Get all 'service' type mapped resources and try to map
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
				serviceKeys = append(serviceKeys, key)
			}
		}

		if obj.RawObj != nil {
			service = *obj.RawObj.(*core_v1.Service).DeepCopy()
		}
		if obj.UpdatedRawObj != nil {
			updatedService = *obj.UpdatedRawObj.(*core_v1.Service).DeepCopy()
		}

		for _, serviceKey := range serviceKeys {
			mappedResource, err := getObjectFromStore(serviceKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//Check if we have to update service
			if len(mappedResource.Kube.Services) > 0 {
				for _, mappedService := range mappedResource.Kube.Services {
					if mappedService.Name == service.Name {
						//Update the service
						mappedService = updatedService

						return MapResult{
							Action:         "Updated",
							Key:            serviceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}
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

										return MapResult{
											Action:         "Updated",
											Key:            serviceKey,
											IsMapped:       true,
											MappedResource: mappedResource,
										}, nil
									}
								}
							}
						}
					}
				}
			}
		}
	case *apps_v1beta2.Deployment:
		var deployment, updatedDeployment apps_v1beta2.Deployment

		//get service from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If deployment has to map with service then they must have same match labels.
			//Get all 'service' type mapped resources and try to map
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
				serviceKeys = append(serviceKeys, key)
			}
		}

		if obj.RawObj != nil {
			deployment = *obj.RawObj.(*apps_v1beta2.Deployment).DeepCopy()
		}
		if obj.UpdatedRawObj != nil {
			updatedDeployment = *obj.UpdatedRawObj.(*apps_v1beta2.Deployment).DeepCopy()
		}

		for _, serviceKey := range serviceKeys {
			mappedResource, err := getObjectFromStore(serviceKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if deployment match to any of service
			if len(mappedResource.Kube.Services) > 0 {
				for _, service := range mappedResource.Kube.Services {
					if reflect.DeepEqual(deployment.Spec.Selector.MatchLabels, service.Spec.Selector) {
						//deployment is matched with this service. Add deployment to this mapped resource
						for _, mappedDeployment := range mappedResource.Kube.Deployments {
							if deployment.UID == mappedDeployment.UID {
								//Deployment exists. Must have been updated
								mappedDeployment = updatedDeployment
							}

							return MapResult{
								Action:         "Updated",
								Key:            serviceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}

						//If its new deployment, add it.
						mappedResource.Kube.Deployments = append(mappedResource.Kube.Deployments, deployment)

						return MapResult{
							Action:         "Added",
							Key:            serviceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}
			}
		}

	case *ext_v1beta1.ReplicaSet:
		var replicaSet, updatedReplicaSet ext_v1beta1.ReplicaSet

		//get service from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with service then it may or may not have same name as service.
			//Get all 'service' type mapped resources and try to map
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
				serviceKeys = append(serviceKeys, key)
			}
		}

		if obj.RawObj != nil {
			replicaSet = *obj.RawObj.(*ext_v1beta1.ReplicaSet).DeepCopy()
		}
		if obj.UpdatedRawObj != nil {
			updatedReplicaSet = *obj.UpdatedRawObj.(*ext_v1beta1.ReplicaSet).DeepCopy()
		}

		for _, serviceKey := range serviceKeys {
			mappedResource, err := getObjectFromStore(serviceKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//Check Deployment followed by service
			//See if pod matched to any of RS
			if len(mappedResource.Kube.Deployments) > 0 {
				for _, deployment := range mappedResource.Kube.Deployments {
					for _, replicaSetOwnerReference := range replicaSet.OwnerReferences {
						if replicaSetOwnerReference.Name == deployment.Name {
							//rs is matched with this Deployment. Add rs to this mapped resource
							for _, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
								if replicaSet.UID == mappedReplicaSet.UID {
									//RS exists. Must have been updated
									mappedReplicaSet = updatedReplicaSet
								}

								return MapResult{
									Action:         "Updated",
									Key:            serviceKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								}, nil
							}

							//If its new pod, add it.
							mappedResource.Kube.ReplicaSets = append(mappedResource.Kube.ReplicaSets, replicaSet)

							return MapResult{
								Action:         "Added",
								Key:            serviceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}
					}
				}
			}

			//Match for service
			if len(mappedResource.Kube.Services) > 0 {
				for _, service := range mappedResource.Kube.Services {
					isMatching := false
					for svcKey, svcValue := range service.Spec.Selector {
						if val, ok := replicaSet.Spec.Selector.MatchLabels[svcKey]; ok {
							if val == svcValue {
								isMatching = true
							} else {
								isMatching = false
							}
						}
					}
					if isMatching {
						//rs is matched with this Deployment. Add rs to this mapped resource
						for _, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
							if replicaSet.UID == mappedReplicaSet.UID {
								//RS exists. Must have been updated
								mappedReplicaSet = updatedReplicaSet
							}

							return MapResult{
								Action:         "Updated",
								Key:            serviceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}

						//If its new pod, add it.
						mappedResource.Kube.ReplicaSets = append(mappedResource.Kube.ReplicaSets, replicaSet)

						return MapResult{
							Action:         "Added",
							Key:            serviceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}
			}
		}

	case *core_v1.Pod:
		//get service from store
		keys := store.ListKeys()
		for _, key := range keys {
			//If pod has to map with service then it may or may not have same name as service.
			//Get all 'service' type mapped resources and try to map
			if strings.Split(key, "/")[0] == obj.Namespace && strings.Split(key, "/")[1] == "service" {
				serviceKeys = append(serviceKeys, key)
			}
		}

		var pod, updatedPod core_v1.Pod

		if obj.RawObj != nil {
			pod = *obj.RawObj.(*core_v1.Pod).DeepCopy()
		}
		if obj.UpdatedRawObj != nil {
			updatedPod = *obj.UpdatedRawObj.(*core_v1.Pod).DeepCopy()
		}

		for _, serviceKey := range serviceKeys {
			mappedResource, err := getObjectFromStore(serviceKey, store)
			if err != nil {
				return MapResult{}, err
			}

			//See if pod is matching to any of RS in this mapped resources
			if len(mappedResource.Kube.ReplicaSets) > 0 {
				for _, replicaSet := range mappedResource.Kube.ReplicaSets {
					for _, podOwnerReference := range pod.OwnerReferences {
						if podOwnerReference.Name == replicaSet.Name {
							//pod is matched with this RS. Add pod to this mapped resource
							for _, mappedPod := range mappedResource.Kube.Pods {
								if pod.UID == mappedPod.UID {
									//pod exists. Must have been updated
									mappedPod = updatedPod
								}

								return MapResult{
									Action:         "Updated",
									Key:            serviceKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								}, nil
							}

							//If its new pod, add it.
							mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)

							return MapResult{
								Action:         "Added",
								Key:            serviceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}
					}
				}
			}
			//Match for deployment
			if len(mappedResource.Kube.Deployments) > 0 {
				for _, deployment := range mappedResource.Kube.Deployments {
					isMatching := false
					for depKey, depValue := range deployment.Spec.Selector.MatchLabels {
						if val, ok := pod.Labels[depKey]; ok {
							if val == depValue {
								isMatching = true
							} else {
								isMatching = false
							}
						}
					}
					if isMatching {
						for _, mappedPod := range mappedResource.Kube.Pods {
							if pod.UID == mappedPod.UID {
								//pod exists. Must have been updated
								mappedPod = updatedPod
							}

							return MapResult{
								Action:         "Updated",
								Key:            serviceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}

						//If its new pod, add it.
						mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)

						return MapResult{
							Action:         "Added",
							Key:            serviceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}
			}
			//Match for service
			if len(mappedResource.Kube.Services) > 0 {
				for _, service := range mappedResource.Kube.Services {
					isMatching := false
					for svcKey, svcValue := range service.Spec.Selector {
						if val, ok := pod.Labels[svcKey]; ok {
							if val == svcValue {
								isMatching = true
							} else {
								isMatching = false
							}
						}
					}
					if isMatching {
						for _, mappedPod := range mappedResource.Kube.Pods {
							if pod.UID == mappedPod.UID {
								//pod exists. Must have been updated
								mappedPod = updatedPod
							}

							return MapResult{
								Action:         "Updated",
								Key:            serviceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}

						//If its new pod, add it.
						mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)

						return MapResult{
							Action:         "Added",
							Key:            serviceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
						}, nil
					}
				}
			}
		}
	default:
		return MapResult{}, fmt.Errorf("Object to be mapped is not supported")
	}

	return MapResult{
		IsMapped: false,
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
		if result.IsMapped {
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
			} else {
				//If key is not present then its new mapped resource.
				//Add new individual mapped resource to store
				err := store.Add(result.MappedResource)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}
