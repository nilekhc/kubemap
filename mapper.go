package kubemap

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	apps_v1beta2 "k8s.io/api/apps/v1beta2"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
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
		mappedIngress, err := mapIngressObj(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return mappedIngress, nil
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
	case "replicaset":
		mappedReplicaSet, err := mapReplicaSetObj(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return []MapResult{
			mappedReplicaSet,
		}, nil
	case "pod":
		mappedPod, err := mapPodObj(obj, store)
		if err != nil {
			return []MapResult{}, err
		}

		return []MapResult{
			mappedPod,
		}, nil
	}

	return []MapResult{}, fmt.Errorf("Resource type '%s' is not supported for mapping", obj.ResourceType)
}

func mapIngressObj(obj ResourceEvent, store cache.Store) ([]MapResult, error) {
	var ingress ext_v1beta1.Ingress
	var namespaceKeys, ingressBackendServices []string
	var mapResults []MapResult

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

		keys := store.ListKeys()
		for _, key := range keys {
			if len(strings.Split(key, "/")) > 0 {
				if strings.Split(key, "/")[0] == obj.Namespace {
					namespaceKeys = append(namespaceKeys, key)
				}
			}
		}

		isMatched := false
		for _, namespaceKey := range namespaceKeys {
			metaIdentifierString := strings.Split(namespaceKey, "/")[1]
			metaIdentifier := MetaIdentifier{}

			json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)

			for _, ingressBackendService := range ingressBackendServices {
				//Try matching with Service
				for _, serviceName := range metaIdentifier.ServicesIdentifier.Names {
					if serviceName == ingressBackendService {
						//Get object
						mappedResource, _ := getObjectFromStore(namespaceKey, store)

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
				},
			)
		}
		return mapResults, nil
	}
	return []MapResult{}, nil
}

func ingressCheck(mappedResource MappedResource, serviceName string, namespaceKeys []string, store cache.Store) (MappedResource, []string) {
	var oldIngressDeleteKeys []string
	for _, namespaceKey := range namespaceKeys {
		metaIdentifierString := strings.Split(namespaceKey, "/")[1]
		metaIdentifier := MetaIdentifier{}

		json.Unmarshal([]byte(metaIdentifierString), &metaIdentifier)
		if metaIdentifier.DeploymentsIdentifier.MatchLabels == nil && metaIdentifier.PodsIdentifier == nil && metaIdentifier.ReplicaSetsIdentifier == nil && metaIdentifier.ServicesIdentifier.MatchLabels == nil && metaIdentifier.IngressBackendServicesIdentifier != nil {
			//Its an object with just ingress
			for _, ingressBackendService := range metaIdentifier.IngressBackendServicesIdentifier {
				if ingressBackendService == serviceName {
					//This ingress belongs to this service. Add it
					ingressMappedResource, _ := getObjectFromStore(namespaceKey, store)
					for _, loneIngress := range ingressMappedResource.Kube.Ingresses {
						mappedResource.Kube.Ingresses = append(mappedResource.Kube.Ingresses, loneIngress)
					}
					oldIngressDeleteKeys = append(oldIngressDeleteKeys, namespaceKey)
				}
			}
		}
	}

	return mappedResource, oldIngressDeleteKeys
}

func mapServiceObj(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var service core_v1.Service
	var namespaceKeys []string

	if obj.Event != nil {
		service = *obj.Event.(*core_v1.Service).DeepCopy()

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
			for _, svcID := range metaIdentifier.ServicesIdentifier.MatchLabels {
				if reflect.DeepEqual(service.Spec.Selector, svcID) {
					//Service and deployment matches. Add service to this mapped resource
					mappedResource, _ := getObjectFromStore(namespaceKey, store)

					for i, mappedService := range mappedResource.Kube.Services {
						if mappedService.Name == service.Name {
							mappedResource.Kube.Services[i] = service

							newMappedResource, deleteKeys := ingressCheck(mappedResource, service.Name, namespaceKeys, store)
							deleteKeys = append(deleteKeys, namespaceKey)
							deleteKeys = removeDuplicateStrings(deleteKeys)

							return MapResult{
								Action:         "Updated",
								DeleteKeys:     deleteKeys,
								IsMapped:       true,
								MappedResource: newMappedResource,
							}, nil
						}
					}
				}
			}

			//Try matching with Deployment
			for _, depID := range metaIdentifier.DeploymentsIdentifier.MatchLabels {
				if reflect.DeepEqual(service.Spec.Selector, depID) {
					//Service and deployment matches. Add service to this mapped resource
					mappedResource, _ := getObjectFromStore(namespaceKey, store)

					for i, mappedService := range mappedResource.Kube.Services {
						if mappedService.Name == service.Name {
							mappedResource.Kube.Services[i] = service

							newMappedResource, deleteKeys := ingressCheck(mappedResource, service.Name, namespaceKeys, store)
							deleteKeys = append(deleteKeys, namespaceKey)
							deleteKeys = removeDuplicateStrings(deleteKeys)

							return MapResult{
								Action:         "Updated",
								DeleteKeys:     deleteKeys,
								IsMapped:       true,
								MappedResource: newMappedResource,
							}, nil
						}
					}

					mappedResource.Kube.Services = append(mappedResource.Kube.Services, service)
					if len(mappedResource.Kube.Services) < 2 { //Set Common Label to service name.
						mappedResource.CommonLabel = service.Name
					}

					newMappedResource, deleteKeys := ingressCheck(mappedResource, service.Name, namespaceKeys, store)
					deleteKeys = append(deleteKeys, namespaceKey)
					deleteKeys = removeDuplicateStrings(deleteKeys)

					return MapResult{
						Action:         "Updated",
						DeleteKeys:     deleteKeys,
						IsMapped:       true,
						MappedResource: newMappedResource,
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
					mappedResource, _ := getObjectFromStore(namespaceKey, store)

					for i, mappedService := range mappedResource.Kube.Services {
						if mappedService.Name == service.Name {
							mappedResource.Kube.Services[i] = service

							newMappedResource, deleteKeys := ingressCheck(mappedResource, service.Name, namespaceKeys, store)
							deleteKeys = append(deleteKeys, namespaceKey)
							deleteKeys = removeDuplicateStrings(deleteKeys)

							return MapResult{
								Action:         "Updated",
								DeleteKeys:     deleteKeys,
								IsMapped:       true,
								MappedResource: newMappedResource,
							}, nil
						}
					}

					mappedResource.Kube.Services = append(mappedResource.Kube.Services, service)
					if len(mappedResource.Kube.Services) < 2 { //Set Common Label to service name.
						mappedResource.CommonLabel = service.Name
					}
					newMappedResource, deleteKeys := ingressCheck(mappedResource, service.Name, namespaceKeys, store)
					deleteKeys = append(deleteKeys, namespaceKey)
					deleteKeys = removeDuplicateStrings(deleteKeys)

					return MapResult{
						Action:         "Updated",
						DeleteKeys:     deleteKeys,
						IsMapped:       true,
						MappedResource: newMappedResource,
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
					mappedResource, _ := getObjectFromStore(namespaceKey, store)

					for i, mappedService := range mappedResource.Kube.Services {
						if mappedService.Name == service.Name {
							mappedResource.Kube.Services[i] = service

							newMappedResource, deleteKeys := ingressCheck(mappedResource, service.Name, namespaceKeys, store)
							deleteKeys = append(deleteKeys, namespaceKey)
							deleteKeys = removeDuplicateStrings(deleteKeys)

							return MapResult{
								Action:         "Updated",
								DeleteKeys:     deleteKeys,
								IsMapped:       true,
								MappedResource: newMappedResource,
							}, nil
						}
					}

					mappedResource.Kube.Services = append(mappedResource.Kube.Services, service)
					if len(mappedResource.Kube.Services) < 2 { //Set Common Label to service name.
						mappedResource.CommonLabel = service.Name
					}
					newMappedResource, deleteKeys := ingressCheck(mappedResource, service.Name, namespaceKeys, store)
					deleteKeys = append(deleteKeys, namespaceKey)
					deleteKeys = removeDuplicateStrings(deleteKeys)

					return MapResult{
						Action: "Updated",
						// Key:            namespaceKey,
						DeleteKeys:     deleteKeys,
						IsMapped:       true,
						MappedResource: newMappedResource,
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

		newMappedResourceWithIngress, deleteKeys := ingressCheck(newMappedService, service.Name, namespaceKeys, store)
		deleteKeys = removeDuplicateStrings(deleteKeys)

		return MapResult{
			Action:         "Added",
			IsMapped:       true,
			DeleteKeys:     deleteKeys,
			MappedResource: newMappedResourceWithIngress,
		}, nil
	}
	return MapResult{}, nil
}

func mapDeploymentObj(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var deployment apps_v1beta2.Deployment
	var namespaceKeys []string

	if obj.Event != nil {
		deployment = *obj.Event.(*apps_v1beta2.Deployment).DeepCopy()

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
			for _, svcID := range metaIdentifier.ServicesIdentifier.MatchLabels {
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
			for _, depID := range metaIdentifier.DeploymentsIdentifier.MatchLabels {
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

			//Try matching with Replica set
			for _, rsID := range metaIdentifier.ReplicaSetsIdentifier {
				for _, ownerReference := range rsID.OwnerReferences {
					if ownerReference == deployment.Name {
						//Deployment and RS matches. Add deployment to this mapped resource
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
						if len(mappedResource.Kube.Deployments) < 2 { //Set Common Label to deployment name.
							mappedResource.CommonLabel = deployment.Name
						}
						return MapResult{
							Action:         "Updated",
							Key:            namespaceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
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
					if len(mappedResource.Kube.Deployments) < 2 { //Set Common Label to deployment name.
						mappedResource.CommonLabel = deployment.Name
					}
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
	return MapResult{}, nil
}

func mapPodObj(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var pod core_v1.Pod
	var namespaceKeys []string

	if obj.Event != nil {
		pod = *obj.Event.(*core_v1.Pod).DeepCopy()

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
					mappedResource, _ := getObjectFromStore(namespaceKey, store)

					for i, mappedPod := range mappedResource.Kube.Pods {
						if mappedPod.Name == pod.Name {
							mappedResource.Kube.Pods[i] = pod

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}
					}

					mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)
					return MapResult{
						Action:         "Updated",
						Key:            namespaceKey,
						IsMapped:       true,
						MappedResource: mappedResource,
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
					mappedResource, _ := getObjectFromStore(namespaceKey, store)

					for i, mappedPod := range mappedResource.Kube.Pods {
						if mappedPod.Name == pod.Name {
							mappedResource.Kube.Pods[i] = pod

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}
					}

					mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)
					return MapResult{
						Action:         "Updated",
						Key:            namespaceKey,
						IsMapped:       true,
						MappedResource: mappedResource,
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
					mappedResource, _ := getObjectFromStore(namespaceKey, store)

					for i, mappedPod := range mappedResource.Kube.Pods {
						if mappedPod.Name == pod.Name {
							mappedResource.Kube.Pods[i] = pod

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}
					}

					mappedResource.Kube.Pods = append(mappedResource.Kube.Pods, pod)
					return MapResult{
						Action:         "Updated",
						Key:            namespaceKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				}

				//Try matching with Pod
				for _, podID := range metaIdentifier.PodsIdentifier {
					if reflect.DeepEqual(pod.Labels, podID.MatchLabels) {
						//Service and deployment matches. Add service to this mapped resource
						mappedResource, _ := getObjectFromStore(namespaceKey, store)

						for i, mappedPod := range mappedResource.Kube.Pods {
							if mappedPod.Name == pod.Name {
								mappedResource.Kube.Pods[i] = pod

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
		}, nil
	}

	//Handle Delete
	if obj.Event == "DELETED" {

	}
	return MapResult{}, nil
}

func mapReplicaSetObj(obj ResourceEvent, store cache.Store) (MapResult, error) {
	var replicaSet ext_v1beta1.ReplicaSet
	var namespaceKeys []string

	if obj.Event != nil {
		replicaSet = *obj.Event.(*ext_v1beta1.ReplicaSet).DeepCopy()

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
						mappedResource, _ := getObjectFromStore(namespaceKey, store)

						for i, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
							if mappedReplicaSet.Name == replicaSet.Name {
								mappedResource.Kube.ReplicaSets[i] = replicaSet

								return MapResult{
									Action:         "Updated",
									Key:            namespaceKey,
									IsMapped:       true,
									MappedResource: mappedResource,
								}, nil
							}
						}

						mappedResource.Kube.ReplicaSets = append(mappedResource.Kube.ReplicaSets, replicaSet)
						return MapResult{
							Action:         "Updated",
							Key:            namespaceKey,
							IsMapped:       true,
							MappedResource: mappedResource,
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
					mappedResource, _ := getObjectFromStore(namespaceKey, store)

					for i, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
						if mappedReplicaSet.Name == replicaSet.Name {
							mappedResource.Kube.ReplicaSets[i] = replicaSet

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
							}, nil
						}
					}

					mappedResource.Kube.ReplicaSets = append(mappedResource.Kube.ReplicaSets, replicaSet)
					return MapResult{
						Action:         "Updated",
						Key:            namespaceKey,
						IsMapped:       true,
						MappedResource: mappedResource,
					}, nil
				}
			}

			//Try matching with Replica set
			for _, rsID := range metaIdentifier.ReplicaSetsIdentifier {
				if reflect.DeepEqual(replicaSet.Spec.Selector.MatchLabels, rsID.MatchLabels) {
					//Service and deployment matches. Add service to this mapped resource
					mappedResource, _ := getObjectFromStore(namespaceKey, store)

					for i, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
						if mappedReplicaSet.Name == replicaSet.Name {
							mappedResource.Kube.ReplicaSets[i] = replicaSet

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
					mappedResource, _ := getObjectFromStore(namespaceKey, store)

					for i, mappedReplicaSet := range mappedResource.Kube.ReplicaSets {
						if mappedReplicaSet.Name == replicaSet.Name {
							mappedResource.Kube.ReplicaSets[i] = replicaSet

							return MapResult{
								Action:         "Updated",
								Key:            namespaceKey,
								IsMapped:       true,
								MappedResource: mappedResource,
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
		}, nil

	}

	return MapResult{}, nil
}
