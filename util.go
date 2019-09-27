package kubemap

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	apps_v1beta1 "k8s.io/api/apps/v1beta1"
	apps_v1beta2 "k8s.io/api/apps/v1beta2"
	autoscaling_v1 "k8s.io/api/autoscaling/v1"
	batch_v1 "k8s.io/api/batch/v1"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

// ObjectMetaData returns metadata of a given k8s object
func objectMetaData(obj interface{}) meta_v1.ObjectMeta {
	//object := obj.(type)
	// switch object {
	switch object := obj.(type) {
	// case *apps_v1beta1.Deployment:
	// 	return object.ObjectMeta
	case *apps_v1beta2.Deployment:
		return object.ObjectMeta
	case *core_v1.ReplicationController:
		return object.ObjectMeta
	case *ext_v1beta1.ReplicaSet:
		return object.ObjectMeta
	case *apps_v1beta1.StatefulSet:
		return object.ObjectMeta
	case *ext_v1beta1.DaemonSet:
		return object.ObjectMeta
	case *core_v1.Service:
		return object.ObjectMeta
	case *core_v1.Pod:
		return object.ObjectMeta
	case *batch_v1.Job:
		return object.ObjectMeta
	case *core_v1.PersistentVolume:
		return object.ObjectMeta
	case *core_v1.PersistentVolumeClaim:
		return object.ObjectMeta
	case *core_v1.Namespace:
		return object.ObjectMeta
	case *core_v1.Secret:
		return object.ObjectMeta
	case *ext_v1beta1.Ingress:
		return object.ObjectMeta
	case *core_v1.Event:
		return object.ObjectMeta
	case *core_v1.ConfigMap:
		return object.ObjectMeta
	case *autoscaling_v1.HorizontalPodAutoscaler:
		return object.ObjectMeta
	}
	var objectMeta meta_v1.ObjectMeta
	return objectMeta
}

//RemoveDuplicateStrings returns unique string slice
func removeDuplicateStrings(elements []string) []string {
	// Use map to record duplicates as we find them.
	encountered := map[string]bool{}
	result := []string{}

	for v := range elements {
		if encountered[elements[v]] == true {
			// Do not add duplicate.
		} else {
			// Record this element as an encountered element.
			encountered[elements[v]] = true
			// Append to result slice.
			result = append(result, elements[v])
		}
	}
	// Return the new slice.
	return result
}

//CopyMappedResource dep copies an object to create new one to avoid pointer references.
//This helps to keep store Thread safe even for Get operations
func copyMappedResource(resource MappedResource) MappedResource {

	copiedMappedResource := MappedResource{}

	for _, item := range resource.Kube.Ingresses {
		copiedMappedResource.Kube.Ingresses = append(copiedMappedResource.Kube.Ingresses, *item.DeepCopy())
	}

	for _, item := range resource.Kube.Services {
		copiedMappedResource.Kube.Services = append(copiedMappedResource.Kube.Services, *item.DeepCopy())
	}

	for _, item := range resource.Kube.Deployments {
		copiedMappedResource.Kube.Deployments = append(copiedMappedResource.Kube.Deployments, *item.DeepCopy())
	}

	for _, item := range resource.Kube.ReplicaSets {
		copiedMappedResource.Kube.ReplicaSets = append(copiedMappedResource.Kube.ReplicaSets, *item.DeepCopy())
	}

	for _, item := range resource.Kube.Pods {
		copiedMappedResource.Kube.Pods = append(copiedMappedResource.Kube.Pods, *item.DeepCopy())
	}

	copiedMappedResource.CommonLabel = resource.CommonLabel
	copiedMappedResource.CurrentType = resource.CurrentType
	copiedMappedResource.Namespace = resource.Namespace

	return copiedMappedResource
}

//MetaIdentifierKeyFunc creates index based on each resource type's identifier like Match Lables, Owner reference etc
func metaResourceKeyFunc(obj interface{}) (string, error) {
	var rsIdentifier, podIdentifier []ChildSet
	var serviceMeta, deploymentMeta MetaSet
	var ingressIdentifier IngressSet

	object := obj.(MappedResource)

	if object.Kube.Ingresses != nil {
		for _, ingress := range object.Kube.Ingresses {
			//Get all services from ingress rules
			for _, ingressRule := range ingress.Spec.Rules {
				if ingressRule.IngressRuleValue.HTTP != nil {
					for _, ingressRuleValueHTTPPath := range ingressRule.IngressRuleValue.HTTP.Paths {
						if ingressRuleValueHTTPPath.Backend.ServiceName != "" {
							ingressIdentifier.IngressBackendServices = append(ingressIdentifier.IngressBackendServices, ingressRuleValueHTTPPath.Backend.ServiceName)
						}
					}
				}
			}
			ingressIdentifier.Names = append(ingressIdentifier.Names, ingress.Name)
		}
		ingressIdentifier.IngressBackendServices = removeDuplicateStrings(ingressIdentifier.IngressBackendServices)
		ingressIdentifier.Names = removeDuplicateStrings(ingressIdentifier.Names)
	}

	if object.Kube.Services != nil {
		for _, service := range object.Kube.Services {
			if service.Spec.Selector != nil {
				serviceMeta.MatchLabels = append(serviceMeta.MatchLabels, service.Spec.Selector)
			}
			serviceMeta.Names = append(serviceMeta.Names, service.Name)
		}
	}

	if object.Kube.Deployments != nil {
		for _, deployment := range object.Kube.Deployments {
			if deployment.Spec.Selector.MatchLabels != nil {
				deploymentMeta.MatchLabels = append(deploymentMeta.MatchLabels, deployment.Spec.Selector.MatchLabels)
			}
			deploymentMeta.Names = append(deploymentMeta.Names, deployment.Name)
		}
	}

	if object.Kube.ReplicaSets != nil {
		var rsOwnerReferences []string
		var rsMatchLables map[string]string

		for _, replicaSet := range object.Kube.ReplicaSets {
			rsOwnerReferences = nil

			if replicaSet.OwnerReferences != nil {
				for _, ownerReference := range replicaSet.OwnerReferences {
					rsOwnerReferences = append(rsOwnerReferences, ownerReference.Name)
				}
			}

			if replicaSet.Spec.Selector.MatchLabels != nil {
				rsMatchLables = replicaSet.Spec.Selector.MatchLabels
			}

			rsIdentifier = append(rsIdentifier, ChildSet{
				Name:            replicaSet.Name,
				OwnerReferences: rsOwnerReferences,
				MatchLabels:     rsMatchLables,
			})
		}
	}

	if object.Kube.Pods != nil {
		var podOwnerReferences []string
		var podMatchLables map[string]string

		for _, pod := range object.Kube.Pods {
			podOwnerReferences = nil

			if pod.OwnerReferences != nil {
				for _, ownerReference := range pod.OwnerReferences {
					podOwnerReferences = append(podOwnerReferences, ownerReference.Name)
				}
			}

			if pod.Labels != nil {
				podMatchLables = pod.Labels
			}

			podIdentifier = append(podIdentifier, ChildSet{
				Name:            pod.Name,
				OwnerReferences: podOwnerReferences,
				MatchLabels:     podMatchLables,
			})
		}
	}

	key := MetaIdentifier{
		IngressIdentifier:     ingressIdentifier,
		ServicesIdentifier:    serviceMeta,
		DeploymentsIdentifier: deploymentMeta,
		ReplicaSetsIdentifier: rsIdentifier,
		PodsIdentifier:        podIdentifier,
	}

	jsonKey, _ := json.Marshal(key)

	storeKey := fmt.Sprintf("%s$%s", object.Namespace, jsonKey)

	base64StoreKey := base64.StdEncoding.EncodeToString([]byte(storeKey))

	// return fmt.Sprintf("%s$%s", object.Namespace, jsonKey), nil
	return base64StoreKey, nil
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

func (m *Mapper) updateStore(results []MapResult, store cache.Store) error {
	for _, result := range results {
		if result.IsMapped && !result.IsStoreUpdated {
			switch result.Action {
			case "Added", "Updated":
				if result.Key != "" {
					//Update object in store
					// existingMappedResource, err := getObjectFromStore(result.Key, store)
					existingMappedResource, err := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(result.Key)), store)

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
						// existingMappedResource, err := getObjectFromStore(deleteKey, store)
						existingMappedResource, err := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(deleteKey)), store)
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
					// existingMappedResource, err := getObjectFromStore(result.Key, store)
					existingMappedResource, err := getObjectFromStore(base64.StdEncoding.EncodeToString([]byte(result.Key)), store)
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
