package kubemap

// import (
// 	"fmt"
// 	"reflect"
// 	"strings"
// 	"sync"

// 	apps_v1beta2 "k8s.io/api/apps/v1beta2"
// 	core_v1 "k8s.io/api/core/v1"
// 	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
// 	"k8s.io/client-go/tools/cache"
// )

// func mapResource(obj interface{}, store cache.Store) error {
// 	object := obj.(ResourceEvent)

// 	lookUpErr := objectLookup(object, store)
// 	if lookUpErr != nil {
// 		// log.Warnf("Error while mapping resource - %v", lookUpErr)
// 		return lookUpErr
// 	}

// 	return nil
// }

// func objectLookup(obj ResourceEvent, store cache.Store) error {
// 	switch obj.ResourceType {
// 	case "ingress":
// 		err := handleIngress(obj, store)
// 		if err != nil {
// 			return err
// 		}
// 	case "service":
// 		err := handleService(obj, store)
// 		if err != nil {
// 			return err
// 		}
// 	case "deployment":
// 		err := handleDeployment(obj, store)
// 		if err != nil {
// 			return err
// 		}
// 	case "replicaset":
// 		err := handleReplicaSet(obj, store)
// 		if err != nil {
// 			return err
// 		}
// 	case "pod":
// 		err := handlePod(obj, store)
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// /*
// *******************************************************************
// *
// *
// Handling Ingress
// *
// *
// *******************************************************************
// */

// func handleIngress(obj ResourceEvent, store cache.Store) error {
// 	wg := sync.WaitGroup{}
// 	errChan := make(chan error)

// 	go func() {
// 		wg.Wait()
// 		close(errChan)
// 	}()

// 	switch obj.EventType {
// 	case "ADDED", "DELETED":
// 		ingressObj := obj.RawObj.(*ext_v1beta1.Ingress).DeepCopy()
// 		ingressServices := getIngressServices(ingressObj)

// 		//Check if service exists in store. If not, create new object in store and add this ingress to it.
// 		//If service exists, check it's ingress for matching one. If found matching ingress update it, if not, add it.

// 		for _, ingressService := range ingressServices {
// 			wg.Add(1)
// 			go upsertIngress(obj, ingressService, store, errChan, &wg)
// 		}
// 	}

// 	for err := range errChan {
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// func getIngressServices(ingressObj *ext_v1beta1.Ingress) []string {
// 	var services []string

// 	//Get default service
// 	if ingressObj.Spec.Backend != nil {
// 		services = append(services, ingressObj.Spec.Backend.ServiceName)
// 	}

// 	//Get all services from ingress rules
// 	for _, ingressRule := range ingressObj.Spec.Rules {
// 		if ingressRule.IngressRuleValue.HTTP != nil {
// 			for _, ingressRuleValueHTTPPath := range ingressRule.IngressRuleValue.HTTP.Paths {
// 				if ingressRuleValueHTTPPath.Backend.ServiceName != "" {
// 					services = append(services, ingressRuleValueHTTPPath.Backend.ServiceName)
// 				}
// 			}
// 		}
// 	}

// 	return removeDuplicateStrings(services)
// }

// func upsertIngress(obj ResourceEvent, ingressService string, store cache.Store, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	//Check if matching service for that ingress exists
// 	storeKey := obj.Namespace + "/service/" + ingressService

// 	//Ingress event Obj
// 	ingressObj := obj.RawObj.(*ext_v1beta1.Ingress).DeepCopy()

// 	item, exists, getErr := store.GetByKey(storeKey)
// 	if getErr != nil {
// 		errChan <- fmt.Errorf("Can't get service object %s from store", storeKey)
// 		return
// 	}

// 	switch obj.EventType {
// 	case "ADDED":
// 		//Service does not exists. Create new ingress obj and add this ingress to it.
// 		if !exists {
// 			//Create new ingress object
// 			mappedResource := MappedResource{}
// 			//mappedResource.CommonLabel = strings.Split(storeKey, "/")[2]
// 			mappedResource.CommonLabel = ingressObj.Name //Deduce common label from Ingress
// 			mappedResource.Ingresses = append(mappedResource.Ingresses, *ingressObj.DeepCopy())
// 			mappedResource.CurrentType = obj.ResourceType
// 			mappedResource.Namespace = obj.Namespace

// 			addErr := store.Add(mappedResource)
// 			if addErr != nil {
// 				errChan <- fmt.Errorf("Can't add object %s to store", mappedResource.CommonLabel+"/"+mappedResource.Namespace)
// 				return
// 			}
// 		} else {
// 			mappedResource := item.(MappedResource)
// 			if len(mappedResource.Ingresses) == 0 {
// 				//This is a service without ingress and has CL set to service name
// 				//Since ingress determines CL, set CL with this ingress name
// 				mappedResource.CommonLabel = ingressObj.Name
// 				//mappedResource.CurrentType = obj.ResourceType //Also update CT with this obj(ingress in this case)
// 			}
// 			mappedResource.Ingresses = append(mappedResource.Ingresses, *ingressObj.DeepCopy())

// 			//Delete and reinsert service obj
// 			delErr := store.Delete(item)
// 			if delErr != nil {
// 				errChan <- fmt.Errorf("Can't delete object %s from store", mappedResource.CommonLabel+"/"+mappedResource.Namespace)
// 				return
// 			}

// 			addErr := store.Add(mappedResource)
// 			if addErr != nil {
// 				errChan <- fmt.Errorf("Can't add object %s to store", mappedResource.CommonLabel+"/"+mappedResource.Namespace)
// 				return
// 			}
// 		}
// 	}
// }

// /*
// *******************************************************************
// *
// *
// Handling Service
// *
// *
// *******************************************************************
// */

// func handleService(obj ResourceEvent, store cache.Store) error {
// 	wg := sync.WaitGroup{}
// 	errChan := make(chan error)
// 	var mapErr error

// 	serviceStoreKey := obj.Namespace + "/" + obj.ResourceType + "/" + obj.Name
// 	var serviceObj core_v1.Service
// 	if obj.EventType == "ADDED" {
// 		serviceObj = *obj.RawObj.(*core_v1.Service).DeepCopy()
// 	}

// 	reacted := false

// 	//Check lone ingress associated with service
// 	wg.Add(1)
// 	go getLoneIngressOfService(serviceStoreKey, obj.EventType, store, serviceObj, &reacted, errChan, &wg)

// 	//Check for lone deployment associated with service
// 	wg.Add(1)
// 	go getLoneDeploymentOfService(serviceStoreKey, obj.EventType, store, serviceObj.DeepCopy(), &reacted, errChan, &wg)

// 	//Check for lone RS associated with service
// 	wg.Add(1)
// 	go getLoneRSOfService(serviceStoreKey, obj.EventType, store, serviceObj.DeepCopy(), &reacted, errChan, &wg)

// 	//Check for lone Pod associated with service
// 	wg.Add(1)
// 	go getLonePodOfService(serviceStoreKey, obj.EventType, store, serviceObj.DeepCopy(), &reacted, errChan, &wg)

// 	go func() {
// 		for err := range errChan {
// 			if err != nil {
// 				mapErr = err
// 				return
// 			}
// 		}
// 	}()

// 	wg.Wait()
// 	close(errChan)

// 	if mapErr != nil {
// 		return mapErr
// 	}

// 	_, exists, err := store.GetByKey(serviceStoreKey)
// 	if err != nil {
// 		return err
// 	}

// 	switch obj.EventType {
// 	case "ADDED":
// 		if !reacted {
// 			//If no lone obj found then create service as lone obj
// 			if !exists {
// 				//Create new object
// 				mappedResource := MappedResource{}
// 				mappedResource.CommonLabel = strings.Split(serviceStoreKey, "/")[2]
// 				mappedResource.Services = append(mappedResource.Services, *serviceObj.DeepCopy())
// 				mappedResource.CurrentType = "service"
// 				mappedResource.Namespace = serviceObj.Namespace

// 				err := store.Add(mappedResource)
// 				if err != nil {
// 					return err
// 				}

// 				reacted = true
// 			}
// 		}
// 	}

// 	return nil
// }

// func getLonePodOfService(serviceStoreKey, eventType string, store cache.Store, serviceObj *core_v1.Service, reacted *bool, errChan chan<- error, wgPod *sync.WaitGroup) {
// 	//List all pod keys
// 	var podStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "pod" {
// 			podStoreKeys = append(podStoreKeys, key)
// 		}
// 	}

// 	//Get matching lone deployment for that service and add to service object and delete deployment
// 	wg := sync.WaitGroup{}
// 	for _, podStoreKey := range podStoreKeys {
// 		wg.Add(1)
// 		go getMatchingPod(podStoreKey, serviceStoreKey, eventType, store, serviceObj.DeepCopy(), reacted, errChan, &wg)
// 	}
// 	wg.Wait()

// 	wgPod.Done()
// }

// func getMatchingPod(podStoreKey, serviceStoreKey, eventType string, store cache.Store, serviceObj *core_v1.Service, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	switch eventType {
// 	case "ADDED", "UPDATED":
// 		//Get Lone Pod Object
// 		podItem, _, podErr := store.GetByKey(podStoreKey)
// 		if podErr != nil {
// 			errChan <- fmt.Errorf("Error while operating on store")
// 			return
// 		}
// 		mappedStoreObj := podItem.(MappedResource)

// 		podMatchedLabels := make(map[string]string)
// 		for _, pod := range mappedStoreObj.Pods {
// 			for svcKey, svcValue := range serviceObj.Spec.Selector {
// 				if val, ok := pod.Labels[svcKey]; ok {
// 					if val == svcValue {
// 						podMatchedLabels[svcKey] = svcValue
// 					}
// 				}
// 			}
// 		}

// 		if reflect.DeepEqual(podMatchedLabels, serviceObj.Spec.Selector) && mappedStoreObj.Namespace == serviceObj.Namespace {
// 			//Found match. Add Pod to this service
// 			//Check if service already exists
// 			svcItem, svcExists, svcErr := store.GetByKey(serviceStoreKey)
// 			if svcErr != nil {
// 				errChan <- fmt.Errorf("Error while operating on store")
// 				return
// 			}

// 			if svcExists {
// 				//Some event has already created this service Obj. Add Ingress to it.
// 				mappedObj := svcItem.(MappedResource)
// 				for _, pod := range mappedStoreObj.Pods {
// 					replaced := false
// 					for _, storePod := range mappedObj.Pods {
// 						if pod.UID == storePod.UID {
// 							//Replace
// 							storePod = *pod.DeepCopy()
// 							replaced = true
// 							break
// 						}
// 					}
// 					if !replaced {
// 						mappedObj.Pods = append(mappedObj.Pods, *pod.DeepCopy())
// 					}
// 				}

// 				delErr := store.Delete(svcItem)
// 				if delErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				addErr := store.Add(mappedObj)
// 				if addErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				//Delete lone ingress
// 				deleteErr := store.Delete(podItem)
// 				if deleteErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				*reacted = true
// 			}
// 		}
// 	}
// }

// func getLoneRSOfService(serviceStoreKey, eventType string, store cache.Store, serviceObj *core_v1.Service, reacted *bool, errChan chan<- error, wgRs *sync.WaitGroup) {
// 	//List all rs keys
// 	var rsStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "replicaset" {
// 			rsStoreKeys = append(rsStoreKeys, key)
// 		}
// 	}
// 	//Get matching lone deployment for that service and add to service object and delete deployment
// 	wg := sync.WaitGroup{}
// 	for _, rsStoreKey := range rsStoreKeys {
// 		wg.Add(1)
// 		go getMatchingRs(rsStoreKey, serviceStoreKey, eventType, store, serviceObj.DeepCopy(), reacted, errChan, &wg)
// 	}
// 	wg.Wait()

// 	wgRs.Done()
// }

// func getMatchingRs(rsStoreKey, serviceStoreKey, eventType string, store cache.Store, serviceObj *core_v1.Service, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	switch eventType {
// 	case "ADDED", "UPDATED":
// 		//Get Lone RS Object
// 		rsItem, _, rsErr := store.GetByKey(rsStoreKey)
// 		if rsErr != nil {
// 			errChan <- fmt.Errorf("Error while operating on store")
// 			return
// 		}
// 		mappedStoreObj := rsItem.(MappedResource)

// 		rsMatchedLabels := make(map[string]string)
// 		for _, rs := range mappedStoreObj.ReplicaSets {
// 			for svcKey, svcValue := range serviceObj.Spec.Selector {
// 				if val, ok := rs.Spec.Selector.MatchLabels[svcKey]; ok {
// 					if val == svcValue {
// 						rsMatchedLabels[svcKey] = svcValue
// 					}
// 				}
// 			}
// 		}

// 		if reflect.DeepEqual(rsMatchedLabels, serviceObj.Spec.Selector) && mappedStoreObj.Namespace == serviceObj.Namespace {
// 			//Found match. Add RS to this service
// 			//Check if service already exists
// 			svcItem, svcExists, svcErr := store.GetByKey(serviceStoreKey)
// 			if svcErr != nil {
// 				errChan <- fmt.Errorf("Error while operating on store")
// 				return
// 			}

// 			if svcExists {
// 				//Some event has already created this service Obj. Add Ingress to it.
// 				mappedObj := svcItem.(MappedResource)
// 				for _, rs := range mappedStoreObj.ReplicaSets {
// 					replaced := false
// 					for _, storeRs := range mappedObj.ReplicaSets {
// 						if rs.UID == storeRs.UID {
// 							//Replace
// 							storeRs = *rs.DeepCopy()
// 							replaced = true
// 							break
// 						}
// 					}
// 					if !replaced {
// 						mappedObj.ReplicaSets = append(mappedObj.ReplicaSets, *rs.DeepCopy())
// 					}
// 				}

// 				delErr := store.Delete(svcItem)
// 				if delErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				addErr := store.Add(mappedObj)
// 				if addErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				//Delete lone ingress
// 				deleteErr := store.Delete(rsItem)
// 				if deleteErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				*reacted = true
// 			}
// 		}
// 	}
// }

// func getLoneDeploymentOfService(serviceStoreKey, eventType string, store cache.Store, serviceObj *core_v1.Service, reacted *bool, errChan chan<- error, wgDep *sync.WaitGroup) {
// 	//List all deployment keys
// 	var deploymentStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "deployment" {
// 			deploymentStoreKeys = append(deploymentStoreKeys, key)
// 		}
// 	}

// 	//Get matching lone deployment for that service and add to service object and delete deployment
// 	wg := sync.WaitGroup{}
// 	for _, deploymentStoreKey := range deploymentStoreKeys {
// 		wg.Add(1)
// 		go getMatchingDeployment(deploymentStoreKey, serviceStoreKey, eventType, store, serviceObj.DeepCopy(), reacted, errChan, &wg)
// 	}
// 	wg.Wait()

// 	wgDep.Done()
// }

// func getMatchingDeployment(deploymentStoreKey, serviceStoreKey, eventType string, store cache.Store, serviceObj *core_v1.Service, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	switch eventType {
// 	case "ADDED", "UPDATED":
// 		//Get Lone Deployment Object
// 		depItem, _, depErr := store.GetByKey(deploymentStoreKey)
// 		if depErr != nil {
// 			errChan <- fmt.Errorf("Can't get deployment")
// 			return
// 		}
// 		mapped := false
// 		mappedStoreObj := depItem.(MappedResource)

// 		for _, deployment := range mappedStoreObj.Deployments {
// 			if reflect.DeepEqual(deployment.Spec.Selector.MatchLabels, serviceObj.Spec.Selector) && deployment.Namespace == serviceObj.Namespace {
// 				//Found match. Add deployment to this service
// 				//Check if service already exists
// 				svcItem, svcExists, svcErr := store.GetByKey(serviceStoreKey)
// 				if svcErr != nil {
// 					errChan <- fmt.Errorf("Can't get service")
// 					return
// 				}

// 				if svcExists {
// 					//Some event has already created this service Obj. Add Ingress to it.
// 					mappedObj := svcItem.(MappedResource)

// 					replaced := false
// 					for _, dep := range mappedObj.Deployments {
// 						if dep.UID == deployment.UID {
// 							dep = *deployment.DeepCopy()
// 							replaced = true
// 							break
// 						}
// 					}
// 					if !replaced {
// 						mappedObj.Deployments = append(mappedObj.Deployments, *deployment.DeepCopy())
// 					}

// 					//Delete and reinsert to ensure Thread Safety
// 					delErr := store.Delete(svcItem)
// 					if delErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					addErr := store.Add(mappedObj)
// 					if addErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					mapped = true
// 					*reacted = true
// 				}
// 			}
// 		}
// 		if mapped {
// 			//Delete lone deployment
// 			deleteErr := store.Delete(depItem)
// 			if deleteErr != nil {
// 				errChan <- fmt.Errorf("Error while operating on store")
// 				return
// 			}
// 		}
// 	}
// }

// func getLoneIngressOfService(serviceStoreKey, eventType string, store cache.Store, serviceObj core_v1.Service, reacted *bool, errChan chan<- error, wgIgs *sync.WaitGroup) {
// 	defer wgIgs.Done()

// 	//List all ingress keys
// 	var ingressStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "ingress" {
// 			ingressStoreKeys = append(ingressStoreKeys, key)
// 		}
// 	}

// 	//Get matching lone ingress for that service and add to service object and delete ingress
// 	wg := sync.WaitGroup{}
// 	for _, ingressStoreKey := range ingressStoreKeys {
// 		wg.Add(1)
// 		go getMatchingIngress(ingressStoreKey, serviceStoreKey, eventType, store, serviceObj, reacted, errChan, &wg)
// 	}
// 	wg.Wait()
// }

// func getMatchingIngress(ingressStoreKey, serviceStoreKey, eventType string, store cache.Store, serviceObj core_v1.Service, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	switch eventType {
// 	case "ADDED":
// 		//Get Lone Ingress Object
// 		ingressItem, ingressExists, ingressErr := store.GetByKey(ingressStoreKey)
// 		if ingressErr != nil {
// 			errChan <- fmt.Errorf("Can't get ingress")
// 			return
// 		}

// 		if ingressExists {
// 			mappedStoreObj := ingressItem.(MappedResource)
// 			mapped := false

// 			//Get all services from ingress rules
// 			if mappedStoreObj.Ingresses != nil {
// 				for _, igs := range mappedStoreObj.Ingresses {
// 					for _, ingressRule := range igs.Spec.Rules {
// 						if ingressRule.IngressRuleValue.HTTP != nil {
// 							for _, ingressRuleValueHTTPPath := range ingressRule.IngressRuleValue.HTTP.Paths {
// 								if ingressRuleValueHTTPPath.Backend.ServiceName != "" {
// 									if ingressRuleValueHTTPPath.Backend.ServiceName == serviceObj.Name && igs.Namespace == serviceObj.Namespace {
// 										// Add this ingress to this service
// 										// Check if service already exists
// 										svcItem, svcExists, svcErr := store.GetByKey(serviceStoreKey)
// 										if svcErr != nil {
// 											errChan <- fmt.Errorf("Can't get service")
// 											return
// 										}

// 										if svcExists {
// 											//Some event has already created this service Obj. Add Ingress to it.
// 											mappedResource := svcItem.(MappedResource)
// 											existingMappedStoreService := copyMappedResource(mappedResource)

// 											if len(mappedResource.Ingresses) == 0 {
// 												//This is a service without ingress and has CL set to service name
// 												//Since ingress determines CL, set CL with this ingress name
// 												mappedResource.CommonLabel = igs.Name
// 												//mappedResource.CurrentType = "ingress" //Also update CT with this obj(ingress in this case)
// 											}

// 											replaced := false
// 											for _, ingress := range existingMappedStoreService.Ingresses {
// 												if ingress.UID == igs.UID {
// 													//Its an existing ingress replace it
// 													ingress = *igs.DeepCopy()
// 													replaced = true
// 												}
// 											}
// 											if !replaced {
// 												//Its different Ingress. Add it
// 												existingMappedStoreService.Ingresses = append(existingMappedStoreService.Ingresses, *igs.DeepCopy())
// 											}

// 											//Delete and reinsert to ensure Thread Safety
// 											delErr := store.Delete(svcItem)
// 											if delErr != nil {
// 												errChan <- fmt.Errorf("Can't delete service")
// 												return
// 											}

// 											addErr := store.Add(existingMappedStoreService)
// 											if addErr != nil {
// 												errChan <- fmt.Errorf("Can't add service")
// 												return
// 											}

// 											mapped = true
// 											*reacted = true
// 										} else {
// 											//This service does not exists. Create new obj
// 											newMappedStoreObj := MappedResource{}
// 											//newMappedStoreObj.CommonLabel = strings.Split(serviceStoreKey, "/")[2]
// 											newMappedStoreObj.CommonLabel = igs.Name
// 											newMappedStoreObj.Ingresses = append(newMappedStoreObj.Ingresses, *igs.DeepCopy())
// 											newMappedStoreObj.Services = append(newMappedStoreObj.Services, *serviceObj.DeepCopy())
// 											newMappedStoreObj.Deployments = mappedStoreObj.Deployments
// 											newMappedStoreObj.ReplicaSets = mappedStoreObj.ReplicaSets
// 											newMappedStoreObj.Pods = mappedStoreObj.Pods
// 											newMappedStoreObj.CurrentType = "service"
// 											newMappedStoreObj.Namespace = serviceObj.Namespace
// 											//newMappedStoreObj.CurrentType = "ingress"

// 											addErr := store.Add(newMappedStoreObj)
// 											if addErr != nil {
// 												errChan <- fmt.Errorf("Can't add service")
// 												return
// 											}

// 											mapped = true
// 											*reacted = true
// 										}
// 									}
// 								}
// 							}
// 						}
// 					}

// 				}
// 			}

// 			if mapped {
// 				//Delete lone ingress
// 				deleteErr := store.Delete(ingressItem)
// 				if deleteErr != nil {
// 					errChan <- fmt.Errorf("Can't delete ingress")
// 					return
// 				}
// 			}
// 		}
// 	}
// }

// /*
// *******************************************************************
// *
// *
// Handling Deployment
// *
// *
// *******************************************************************
// */
// func handleDeployment(obj ResourceEvent, store cache.Store) error {
// 	wg := sync.WaitGroup{}
// 	errChan := make(chan error)
// 	var mapErr error

// 	deploymentStoreKey := obj.Namespace + "/" + obj.ResourceType + "/" + obj.Name
// 	var deploymentObj *apps_v1beta2.Deployment
// 	if obj.EventType == "ADDED" {
// 		deploymentObj = obj.RawObj.(*apps_v1beta2.Deployment).DeepCopy()
// 	}

// 	reacted := false

// 	//Get lone service
// 	wg.Add(1)
// 	go getLoneServiceOfDeployment(deploymentStoreKey, obj.EventType, store, deploymentObj.DeepCopy(), &reacted, errChan, &wg)

// 	//Get lone RS
// 	wg.Add(1)
// 	go getLoneRsOfDeployment(deploymentStoreKey, obj.EventType, store, deploymentObj.DeepCopy(), &reacted, errChan, &wg)

// 	//Get lone Pod
// 	wg.Add(1)
// 	go getLonePodOfDeployment(deploymentStoreKey, obj.EventType, store, deploymentObj.DeepCopy(), &reacted, errChan, &wg)

// 	go func() {
// 		for err := range errChan {
// 			if err != nil {
// 				mapErr = err
// 				return
// 			}
// 		}
// 	}()

// 	wg.Wait()
// 	close(errChan)

// 	if mapErr != nil {
// 		return mapErr
// 	}

// 	//If no lone obj found then create deployment as lone obj
// 	_, depExists, err := store.GetByKey(deploymentStoreKey)
// 	if err != nil {
// 		return err
// 	}

// 	switch obj.EventType {
// 	case "ADDED":
// 		if !reacted {
// 			if !depExists {
// 				//Create lone deployment obj
// 				mappedResource := MappedResource{}
// 				mappedResource.CommonLabel = strings.Split(deploymentStoreKey, "/")[2]
// 				mappedResource.Deployments = append(mappedResource.Deployments, *deploymentObj.DeepCopy())
// 				mappedResource.CurrentType = "deployment"
// 				mappedResource.Namespace = deploymentObj.Namespace

// 				err := store.Add(mappedResource)
// 				if err != nil {
// 					return err
// 				}

// 				reacted = true
// 			}
// 		}
// 	}

// 	return nil
// }

// func getLonePodOfDeployment(deploymentStoreKey, eventType string, store cache.Store, deploymentObj *apps_v1beta2.Deployment, reacted *bool, errChan chan<- error, wgPod *sync.WaitGroup) {
// 	//List all pod keys
// 	var podStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "pod" {
// 			podStoreKeys = append(podStoreKeys, key)
// 		}
// 	}

// 	//Get matching lone ingress for that service and add to service object and delete ingress
// 	wg := sync.WaitGroup{}
// 	for _, podStoreKey := range podStoreKeys {
// 		wg.Add(1)
// 		go getMatchingPodOfDeployment(podStoreKey, deploymentStoreKey, eventType, store, deploymentObj.DeepCopy(), reacted, errChan, &wg)
// 	}
// 	wg.Wait()

// 	wgPod.Done()
// }

// func getMatchingPodOfDeployment(podStoreKey, deploymentStoreKey, eventType string, store cache.Store, deploymentObj *apps_v1beta2.Deployment, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	switch eventType {
// 	case "ADDED", "UPDATED":
// 		//Get Lone Pod Object
// 		podItem, _, podErr := store.GetByKey(podStoreKey)
// 		if podErr != nil {
// 			errChan <- fmt.Errorf("Error while operating on store")
// 			return
// 		}
// 		mappedStoreObj := podItem.(MappedResource)

// 		podMatchedLables := make(map[string]string)
// 		if mappedStoreObj.Pods != nil {
// 			for depKey, depValue := range deploymentObj.Spec.Selector.MatchLabels {
// 				if val, ok := mappedStoreObj.Pods[0].Labels[depKey]; ok {
// 					if val == depValue {
// 						podMatchedLables[depKey] = depValue
// 					}
// 				}
// 			}

// 			if reflect.DeepEqual(podMatchedLables, deploymentObj.Spec.Selector.MatchLabels) && mappedStoreObj.Namespace == deploymentObj.Namespace {
// 				//Found match. Add Pod to this deployment
// 				//Check if deployment already exists
// 				depItem, depExists, depErr := store.GetByKey(deploymentStoreKey)
// 				if depErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				if depExists {
// 					//Some event has already created this deployment Obj. Add pod to it.
// 					mappedObj := depItem.(MappedResource)

// 					replaced := false
// 					for _, pod := range mappedObj.Pods {
// 						if pod.UID == mappedStoreObj.Pods[0].UID {
// 							pod = *mappedStoreObj.Pods[0].DeepCopy()
// 							replaced = true
// 							break
// 						}
// 					}

// 					if !replaced {
// 						mappedObj.Pods = append(mappedObj.Pods, *mappedStoreObj.Pods[0].DeepCopy())
// 					}

// 					//UPDATE - Delete and Reinsert
// 					delErr := store.Delete(depItem)
// 					if delErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					addErr := store.Add(mappedObj)
// 					if addErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					//Delete lone pod
// 					deleteErr := store.Delete(podItem)
// 					if deleteErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					*reacted = true
// 				}
// 			}
// 		}
// 	}
// }

// func getLoneRsOfDeployment(deploymentStoreKey, eventType string, store cache.Store, deploymentObj *apps_v1beta2.Deployment, reacted *bool, errChan chan<- error, wgRs *sync.WaitGroup) {
// 	//List all rs keys
// 	var rsStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "replicaset" {
// 			rsStoreKeys = append(rsStoreKeys, key)
// 		}
// 	}

// 	//Get matching lone ingress for that service and add to service object and delete ingress
// 	wg := sync.WaitGroup{}
// 	for _, rsStoreKey := range rsStoreKeys {
// 		wg.Add(1)
// 		go getMatchingRsOfDeployment(rsStoreKey, deploymentStoreKey, eventType, store, deploymentObj.DeepCopy(), reacted, errChan, &wg)
// 	}
// 	wg.Wait()

// 	wgRs.Done()
// }

// func getMatchingRsOfDeployment(rsStoreKey, deploymentStoreKey, eventType string, store cache.Store, deploymentObj *apps_v1beta2.Deployment, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	switch eventType {
// 	case "ADDED", "UPDATED":
// 		//Get Lone RS Object
// 		rsItem, _, rsErr := store.GetByKey(rsStoreKey)
// 		if rsErr != nil {
// 			errChan <- fmt.Errorf("Error while operating on store")
// 			return
// 		}
// 		mappedStoreObj := rsItem.(MappedResource)

// 		rsMatchedLabels := make(map[string]string)
// 		if mappedStoreObj.ReplicaSets != nil {
// 			for depKey, depValue := range deploymentObj.Spec.Selector.MatchLabels {
// 				if val, ok := mappedStoreObj.ReplicaSets[0].Spec.Selector.MatchLabels[depKey]; ok {
// 					if val == depValue {
// 						rsMatchedLabels[depKey] = depValue
// 					}
// 				}
// 			}

// 			if reflect.DeepEqual(rsMatchedLabels, deploymentObj.Spec.Selector.MatchLabels) && mappedStoreObj.Namespace == deploymentObj.Namespace {
// 				//Found match. Add RS to this deployment
// 				//Check if deployment already exists
// 				depItem, depExists, depErr := store.GetByKey(deploymentStoreKey)
// 				if depErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				if depExists {
// 					//Some event has already created this deployment Obj. Add RS to it.
// 					mappedObj := depItem.(MappedResource)

// 					replaced := false
// 					for _, rs := range mappedObj.ReplicaSets {
// 						if rs.UID == mappedStoreObj.ReplicaSets[0].UID {
// 							rs = *mappedStoreObj.ReplicaSets[0].DeepCopy()
// 							replaced = true
// 							break
// 						}
// 					}

// 					if !replaced {
// 						mappedObj.ReplicaSets = append(mappedObj.ReplicaSets, *mappedStoreObj.ReplicaSets[0].DeepCopy())
// 					}

// 					//Delete and reinsert for thread safety
// 					delErr := store.Delete(depItem)
// 					if delErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					addErr := store.Add(mappedObj)
// 					if addErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					//Delete lone rs
// 					deleteErr := store.Delete(rsItem)
// 					if deleteErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					*reacted = true
// 				}
// 			}
// 		}

// 	}
// }

// func getLoneServiceOfDeployment(deploymentStoreKey, eventType string, store cache.Store, deploymentObj *apps_v1beta2.Deployment, reacted *bool, errChan chan<- error, wgSvc *sync.WaitGroup) {
// 	//List all ingress keys
// 	var serviceStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "service" {
// 			serviceStoreKeys = append(serviceStoreKeys, key)
// 		}
// 	}

// 	//Get matching lone ingress for that service and add to service object and delete ingress
// 	wg := sync.WaitGroup{}
// 	for _, serviceStoreKey := range serviceStoreKeys {
// 		wg.Add(1)
// 		go getMatchingServiceOfDeployment(serviceStoreKey, deploymentStoreKey, eventType, store, deploymentObj.DeepCopy(), reacted, errChan, &wg)
// 	}
// 	wg.Wait()

// 	wgSvc.Done()
// }

// func getMatchingServiceOfDeployment(serviceStoreKey, deploymentStoreKey, eventType string, store cache.Store, deploymentObj *apps_v1beta2.Deployment, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	//Get Lone Service Object
// 	svcItem, _, svcErr := store.GetByKey(serviceStoreKey)
// 	if svcErr != nil {
// 		errChan <- fmt.Errorf("Error while operating on store")
// 		return
// 	}

// 	switch eventType {
// 	case "ADDED", "UPDATED":
// 		mappedStoreObj := svcItem.(MappedResource)
// 		existingMappedStoreService := copyMappedResource(mappedStoreObj)

// 		if existingMappedStoreService.Services != nil { //Its a lone object
// 			if reflect.DeepEqual(deploymentObj.Spec.Selector.MatchLabels, existingMappedStoreService.Services[0].Spec.Selector) && existingMappedStoreService.Namespace == deploymentObj.Namespace {
// 				//Found match. Add deployment to this service
// 				replaced := false
// 				for _, dep := range existingMappedStoreService.Deployments {
// 					if dep.UID == deploymentObj.UID {
// 						//UPDATE event
// 						//Its exiting rs object. replace it
// 						dep = *deploymentObj.DeepCopy()
// 						replaced = true
// 						break
// 					}
// 				}

// 				if !replaced {
// 					existingMappedStoreService.Deployments = append(existingMappedStoreService.Deployments, *deploymentObj.DeepCopy())
// 				}

// 				//Delete and reinsert for Thread Safety
// 				delErr := store.Delete(svcItem)
// 				if delErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				addErr := store.Add(existingMappedStoreService)
// 				if addErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				*reacted = true
// 			}
// 		}
// 	case "DELETED":
// 		var newDeploymentSet []apps_v1beta2.Deployment
// 		mappedStoreService := svcItem.(MappedResource)

// 		if mappedStoreService.Namespace == deploymentObj.Namespace {
// 			//Check if deployment to be deleted is present in this mapped service object
// 			for _, dep := range mappedStoreService.Deployments {
// 				if dep.UID != deploymentObj.UID {
// 					newDeploymentSet = append(newDeploymentSet, *dep.DeepCopy())
// 				}
// 			}
// 			mappedStoreService.Deployments = newDeploymentSet

// 			delErr := store.Delete(svcItem)
// 			if delErr != nil {
// 				errChan <- fmt.Errorf("Error while operating on store")
// 				return
// 			}

// 			addErr := store.Add(mappedStoreService)
// 			if addErr != nil {
// 				errChan <- fmt.Errorf("Error while operating on store")
// 				return
// 			}

// 			*reacted = true
// 		}
// 	}
// }

// /*
// *******************************************************************
// *
// *
// Handling Replica Set
// *
// *
// *******************************************************************
// */
// func handleReplicaSet(obj ResourceEvent, store cache.Store) error {
// 	wg := sync.WaitGroup{}
// 	errChan := make(chan error)
// 	var mapErr error

// 	rsStoreKey := obj.Namespace + "/" + obj.ResourceType + "/" + obj.Name
// 	var replicaSetObj *ext_v1beta1.ReplicaSet
// 	if obj.EventType == "ADDED" {
// 		replicaSetObj = obj.RawObj.(*ext_v1beta1.ReplicaSet).DeepCopy()
// 	}

// 	reacted := false

// 	//Get lone service
// 	wg.Add(1)
// 	go getLoneDeploymentOfRs(rsStoreKey, obj.EventType, store, replicaSetObj.DeepCopy(), &reacted, errChan, &wg)

// 	//Get lone RS
// 	wg.Add(1)
// 	go getLoneServiceOfRs(rsStoreKey, obj.EventType, store, replicaSetObj.DeepCopy(), &reacted, errChan, &wg)

// 	//Get lone Pod
// 	wg.Add(1)
// 	go getLonePodOfRs(rsStoreKey, obj.EventType, store, replicaSetObj.DeepCopy(), &reacted, errChan, &wg)

// 	go func() {
// 		for err := range errChan {
// 			if err != nil {
// 				mapErr = err
// 				return
// 			}
// 		}
// 	}()

// 	wg.Wait()
// 	close(errChan)

// 	if mapErr != nil {
// 		return mapErr
// 	}

// 	//Check if lone RS exists
// 	_, rsExists, err := store.GetByKey(rsStoreKey)
// 	if err != nil {
// 		return err
// 	}

// 	switch obj.EventType {
// 	case "ADDED":
// 		if !reacted {
// 			if !rsExists {
// 				//This deployment does not exists. Create new lone rs
// 				mappedObj := MappedResource{}
// 				mappedObj.CommonLabel = strings.Split(rsStoreKey, "/")[2]
// 				mappedObj.ReplicaSets = append(mappedObj.ReplicaSets, *replicaSetObj.DeepCopy())
// 				mappedObj.CurrentType = "replicaset"
// 				mappedObj.Namespace = replicaSetObj.Namespace

// 				err := store.Add(mappedObj)
// 				if err != nil {
// 					return err
// 				}

// 				reacted = true
// 			}
// 		}
// 	}

// 	return nil
// }

// func getLonePodOfRs(rsStoreKey, eventType string, store cache.Store, replicaSetObj *ext_v1beta1.ReplicaSet, reacted *bool, errChan chan<- error, wgPod *sync.WaitGroup) {
// 	//List all pod keys
// 	var podStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "pod" {
// 			podStoreKeys = append(podStoreKeys, key)
// 		}
// 	}

// 	//Get matching lone ingress for that service and add to service object and delete ingress
// 	wg := sync.WaitGroup{}
// 	for _, podStoreKey := range podStoreKeys {
// 		wg.Add(1)
// 		go getMatchingPodOfRs(podStoreKey, rsStoreKey, eventType, store, replicaSetObj.DeepCopy(), reacted, errChan, &wg)
// 	}
// 	wg.Wait()

// 	wgPod.Done()
// }

// func getMatchingPodOfRs(podStoreKey, rsStoreKey, eventType string, store cache.Store, replicaSetObj *ext_v1beta1.ReplicaSet, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	switch eventType {
// 	case "ADDED", "UPDATED":
// 		//Get Lone Pod Object
// 		podItem, _, podErr := store.GetByKey(podStoreKey)
// 		if podErr != nil {
// 			errChan <- fmt.Errorf("Error while operating on store")
// 			return
// 		}
// 		mappedStoreObj := podItem.(MappedResource)

// 		if mappedStoreObj.Pods != nil {
// 			podMatchedLabels := make(map[string]string)
// 			for rsKey, rsValue := range replicaSetObj.Spec.Selector.MatchLabels {
// 				if val, ok := mappedStoreObj.Pods[0].Labels[rsKey]; ok {
// 					if val == rsValue {
// 						podMatchedLabels[rsKey] = rsValue
// 					}
// 				}
// 			}

// 			if reflect.DeepEqual(podMatchedLabels, replicaSetObj.Spec.Selector.MatchLabels) && mappedStoreObj.Namespace == replicaSetObj.Namespace {
// 				//Found match. Add Pod to this rs
// 				//Check if rs already exists
// 				rsItem, rsExists, rsErr := store.GetByKey(rsStoreKey)
// 				if rsErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				if rsExists {
// 					//Some event has already created this rs Obj. Add pod to it.
// 					mappedObj := rsItem.(MappedResource)

// 					replaced := false
// 					for _, pod := range mappedObj.Pods {
// 						if pod.UID == mappedStoreObj.Pods[0].UID {
// 							pod = *mappedStoreObj.Pods[0].DeepCopy()
// 							replaced = true
// 							break
// 						}
// 					}

// 					if !replaced {
// 						mappedObj.Pods = append(mappedObj.Pods, *mappedStoreObj.Pods[0].DeepCopy())
// 					}

// 					//Delete and reinsert
// 					delErr := store.Delete(rsItem)
// 					if delErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					addErr := store.Add(mappedObj)
// 					if addErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					//Delete lone pod
// 					deleteErr := store.Delete(podItem)
// 					if deleteErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					*reacted = true
// 				} else {
// 					//Create new RS and store this pod to it. And then delete lone pod
// 					//This deployment does not exists. Create new lone rs
// 					mappedObj := MappedResource{}
// 					mappedObj.CommonLabel = strings.Split(rsStoreKey, "/")[2]
// 					mappedObj.ReplicaSets = append(mappedObj.ReplicaSets, *replicaSetObj.DeepCopy())
// 					mappedObj.CurrentType = "replicaset"
// 					mappedObj.Pods = append(mappedObj.Pods, *mappedStoreObj.Pods[0].DeepCopy())
// 					mappedObj.Namespace = replicaSetObj.Namespace

// 					addErr := store.Add(mappedObj)
// 					if addErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					//Delete lone pod
// 					deleteErr := store.Delete(podItem)
// 					if deleteErr != nil {
// 						errChan <- fmt.Errorf("Error while operating on store")
// 						return
// 					}

// 					*reacted = true
// 				}
// 			}
// 		}
// 	}
// }

// func getLoneServiceOfRs(rsStoreKey, eventType string, store cache.Store, replicaSetObj *ext_v1beta1.ReplicaSet, reacted *bool, errChan chan<- error, wgSvc *sync.WaitGroup) {
// 	//List all svc keys
// 	var svcStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "service" {
// 			svcStoreKeys = append(svcStoreKeys, key)
// 		}
// 	}

// 	//Get matching lone ingress for that service and add to service object and delete ingress
// 	wg := sync.WaitGroup{}
// 	for _, svcStoreKey := range svcStoreKeys {
// 		wg.Add(1)
// 		go getMatchingServiceOfRs(svcStoreKey, rsStoreKey, eventType, store, replicaSetObj.DeepCopy(), reacted, errChan, &wg)
// 	}
// 	wg.Wait()

// 	wgSvc.Done()
// }

// func getMatchingServiceOfRs(svcStoreKey, rsStoreKey, eventType string, store cache.Store, replicaSetObj *ext_v1beta1.ReplicaSet, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	//Get Lone service Object
// 	svcItem, _, svcErr := store.GetByKey(svcStoreKey)
// 	if svcErr != nil {
// 		errChan <- fmt.Errorf("Error while operating on store")
// 		return
// 	}

// 	switch eventType {
// 	case "ADDED", "UPDATED":
// 		mappedStoreObj := svcItem.(MappedResource)

// 		if mappedStoreObj.Services != nil {
// 			rsMatchedLabels := make(map[string]string)
// 			for svcKey, svcValue := range mappedStoreObj.Services[0].Spec.Selector {
// 				if val, ok := replicaSetObj.Spec.Selector.MatchLabels[svcKey]; ok {
// 					if val == svcValue {
// 						rsMatchedLabels[svcKey] = svcValue
// 					}
// 				}
// 			}

// 			if reflect.DeepEqual(rsMatchedLabels, mappedStoreObj.Services[0].Spec.Selector) && mappedStoreObj.Namespace == replicaSetObj.Namespace {
// 				//Found match. Add RS to this service
// 				replaced := false
// 				for _, rs := range mappedStoreObj.ReplicaSets {
// 					if rs.UID == replicaSetObj.UID {
// 						//UPDATE event
// 						//Its exiting rs object. replace it
// 						rs = *replicaSetObj.DeepCopy()
// 						replaced = true
// 						break
// 					}
// 				}

// 				if !replaced {
// 					//Its new pod. Add it to RS
// 					mappedStoreObj.ReplicaSets = append(mappedStoreObj.ReplicaSets, *replicaSetObj.DeepCopy())
// 				}

// 				//delete and reinsert for thread safety
// 				delErr := store.Delete(svcItem)
// 				if delErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				addErr := store.Add(mappedStoreObj)
// 				if addErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				*reacted = true
// 			}
// 		}
// 	}
// }

// func getLoneDeploymentOfRs(rsStoreKey, eventType string, store cache.Store, replicaSetObj *ext_v1beta1.ReplicaSet, reacted *bool, errChan chan<- error, wgDep *sync.WaitGroup) {
// 	//List all deployment keys
// 	var depStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "deployment" {
// 			depStoreKeys = append(depStoreKeys, key)
// 		}
// 	}

// 	//Get matching lone ingress for that service and add to service object and delete ingress
// 	wg := sync.WaitGroup{}
// 	for _, depStoreKey := range depStoreKeys {
// 		wg.Add(1)
// 		go getMatchingDeploymentOfRs(depStoreKey, rsStoreKey, eventType, store, replicaSetObj.DeepCopy(), reacted, errChan, &wg)
// 	}
// 	wg.Wait()

// 	wgDep.Done()
// }

// func getMatchingDeploymentOfRs(depStoreKey, rsStoreKey, eventType string, store cache.Store, replicaSetObj *ext_v1beta1.ReplicaSet, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	//Get Lone Deployment Object
// 	depItem, _, depErr := store.GetByKey(depStoreKey)
// 	if depErr != nil {
// 		errChan <- fmt.Errorf("Error while operating on store")
// 		return
// 	}

// 	switch eventType {
// 	case "ADDED", "UPDATED":
// 		mappedStoreObj := depItem.(MappedResource)

// 		if mappedStoreObj.Deployments != nil {
// 			rsMatchedLabels := make(map[string]string)
// 			for depKey, depValue := range mappedStoreObj.Deployments[0].Spec.Selector.MatchLabels {
// 				if val, ok := replicaSetObj.Spec.Selector.MatchLabels[depKey]; ok {
// 					if val == depValue {
// 						rsMatchedLabels[depKey] = depValue
// 					}
// 				}
// 			}

// 			if reflect.DeepEqual(rsMatchedLabels, mappedStoreObj.Deployments[0].Spec.Selector.MatchLabels) && mappedStoreObj.Namespace == replicaSetObj.Namespace {
// 				//Found match. Add RS to this deployment
// 				replaced := false
// 				for _, rs := range mappedStoreObj.ReplicaSets {
// 					if rs.UID == replicaSetObj.UID {
// 						//UPDATE event
// 						//Its exiting rs object. replace it
// 						rs = *replicaSetObj.DeepCopy()
// 						replaced = true
// 						break
// 					}
// 				}

// 				if !replaced {
// 					//Its new pod. Add it to RS
// 					mappedStoreObj.ReplicaSets = append(mappedStoreObj.ReplicaSets, *replicaSetObj.DeepCopy())
// 				}

// 				//Delete and reinsert to ensure Thread Safety
// 				delErr := store.Delete(depItem)
// 				if delErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				addErr := store.Add(mappedStoreObj)
// 				if addErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				*reacted = true
// 			}
// 		}
// 	}
// }

// /*
// *******************************************************************
// *
// *
// Handling Pod
// *
// *
// *******************************************************************
// */

// func handlePod(obj ResourceEvent, store cache.Store) error {
// 	wg := sync.WaitGroup{}
// 	errChan := make(chan error)
// 	var mapErr error

// 	podStoreKey := obj.Namespace + "/" + obj.ResourceType + "/" + obj.Name
// 	var podObj *core_v1.Pod
// 	if obj.EventType == "ADDED" {
// 		podObj = obj.RawObj.(*core_v1.Pod).DeepCopy()
// 	}

// 	reacted := false

// 	//Get lone rs
// 	wg.Add(1)
// 	go getLoneRsOfPod(podStoreKey, obj.EventType, store, podObj, &reacted, errChan, &wg)

// 	//Get lone deployment
// 	wg.Add(1)
// 	go getLoneDeploymentOfPod(podStoreKey, obj.EventType, store, podObj, &reacted, errChan, &wg)

// 	//Get lone service
// 	wg.Add(1)
// 	go getLoneServiceOfPod(podStoreKey, obj.EventType, store, podObj, &reacted, errChan, &wg)

// 	go func() {
// 		for err := range errChan {
// 			if err != nil {
// 				mapErr = err
// 				return
// 			}
// 		}
// 	}()

// 	wg.Wait()
// 	close(errChan)

// 	if mapErr != nil {
// 		return mapErr
// 	}

// 	_, podExists, err := store.GetByKey(podStoreKey)
// 	if err != nil {
// 		return err
// 	}

// 	switch obj.EventType {
// 	case "ADDED":
// 		if !reacted {
// 			//If no lone obj found then create pos as lone obj
// 			//Create new obj of pod

// 			if !podExists {
// 				mappedObj := MappedResource{}
// 				mappedObj.CommonLabel = strings.Split(podStoreKey, "/")[2]
// 				mappedObj.Pods = append(mappedObj.Pods, *podObj.DeepCopy())
// 				mappedObj.CurrentType = "pod"
// 				mappedObj.Namespace = podObj.Namespace

// 				err := store.Add(mappedObj)
// 				if err != nil {
// 					return err
// 				}
// 			}
// 		}
// 	}

// 	return nil
// }

// func getLoneRsOfPod(podStoreKey, eventType string, store cache.Store, podObj *core_v1.Pod, reacted *bool, errChan chan<- error, wgRs *sync.WaitGroup) {
// 	//List all pod keys
// 	var rsStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "replicaset" {
// 			rsStoreKeys = append(rsStoreKeys, key)
// 		}
// 	}

// 	//Get matching lone ingress for that service and add to service object and delete ingress
// 	wg := sync.WaitGroup{}
// 	for _, rsStoreKey := range rsStoreKeys {
// 		wg.Add(1)
// 		go getMatchingRsOfPod(rsStoreKey, podStoreKey, eventType, store, podObj.DeepCopy(), reacted, errChan, &wg)
// 	}
// 	wg.Wait()

// 	wgRs.Done()
// }

// func getMatchingRsOfPod(rsStoreKey, podStoreKey, eventType string, store cache.Store, podObj *core_v1.Pod, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	//Get Lone Rs Object
// 	rsItem, _, rsErr := store.GetByKey(rsStoreKey)
// 	if rsErr != nil {
// 		errChan <- fmt.Errorf("Error while operating on store")
// 		return
// 	}

// 	switch eventType {
// 	case "ADDED", "UPDATED":
// 		mappedStoreObj := rsItem.(MappedResource)

// 		podMatchedLables := make(map[string]string)
// 		if mappedStoreObj.ReplicaSets != nil {
// 			for rsKey, rsValue := range mappedStoreObj.ReplicaSets[0].Spec.Selector.MatchLabels {
// 				if val, ok := podObj.Labels[rsKey]; ok {
// 					if val == rsValue {
// 						podMatchedLables[rsKey] = rsValue
// 					}
// 				}
// 			}

// 			if reflect.DeepEqual(podMatchedLables, mappedStoreObj.ReplicaSets[0].Spec.Selector.MatchLabels) && mappedStoreObj.Namespace == podObj.Namespace {
// 				//Found match. Add Pod to this rs
// 				replaced := false
// 				for _, pod := range mappedStoreObj.Pods {
// 					if pod.UID == podObj.UID {
// 						//UPDATE event
// 						//Its exiting pod object. replace it
// 						pod = *podObj.DeepCopy()
// 						replaced = true
// 						break
// 					}
// 				}

// 				if !replaced {
// 					//Its new pod. Add it to RS
// 					mappedStoreObj.Pods = append(mappedStoreObj.Pods, *podObj.DeepCopy())
// 				}

// 				//Delete and reinsert to ensure Thread Safety
// 				delErr := store.Delete(rsItem)
// 				if delErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				addErr := store.Add(mappedStoreObj)
// 				if addErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				*reacted = true
// 			}
// 		}
// 	case "DELETED":
// 		var newPodSet []core_v1.Pod
// 		mappedStoreReplicaSet := rsItem.(MappedResource)

// 		if mappedStoreReplicaSet.Namespace == podObj.Namespace {
// 			//Check if pod to be deleted is present in this mapped deployment object
// 			for _, pod := range mappedStoreReplicaSet.Pods {
// 				if pod.UID != podObj.UID {
// 					newPodSet = append(newPodSet, *pod.DeepCopy())
// 				}
// 			}
// 			mappedStoreReplicaSet.Pods = newPodSet

// 			//Delete and reinsert to ensure Thread Safety
// 			delErr := store.Delete(rsItem)
// 			if delErr != nil {
// 				errChan <- fmt.Errorf("Error while operating on store")
// 				return
// 			}

// 			addErr := store.Add(mappedStoreReplicaSet)
// 			if addErr != nil {
// 				errChan <- fmt.Errorf("Error while operating on store")
// 				return
// 			}

// 			*reacted = true
// 		}
// 	}
// }

// func getLoneDeploymentOfPod(podStoreKey, eventType string, store cache.Store, podObj *core_v1.Pod, reacted *bool, errChan chan<- error, wgDep *sync.WaitGroup) {
// 	//List all deployment keys
// 	var depStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "deployment" {
// 			depStoreKeys = append(depStoreKeys, key)
// 		}
// 	}

// 	//Get matching lone ingress for that service and add to service object and delete ingress
// 	wg := sync.WaitGroup{}
// 	for _, depStoreKey := range depStoreKeys {
// 		wg.Add(1)
// 		go getMatchingDeploymentOfPod(depStoreKey, podStoreKey, eventType, store, podObj.DeepCopy(), reacted, errChan, &wg)
// 	}
// 	wg.Wait()

// 	wgDep.Done()
// }

// func getMatchingDeploymentOfPod(depStoreKey, podStoreKey, eventType string, store cache.Store, podObj *core_v1.Pod, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	//Get Lone deployment Object
// 	depItem, _, depErr := store.GetByKey(depStoreKey)
// 	if depErr != nil {
// 		errChan <- fmt.Errorf("Error while operating on store")
// 		return
// 	}

// 	switch eventType {
// 	case "ADDED", "UPDATED":
// 		mappedStoreObj := depItem.(MappedResource)

// 		podMatchedLables := make(map[string]string)
// 		if mappedStoreObj.Deployments != nil {
// 			for depKey, depValue := range mappedStoreObj.Deployments[0].Spec.Selector.MatchLabels {
// 				if val, ok := podObj.Labels[depKey]; ok {
// 					if val == depValue {
// 						podMatchedLables[depKey] = depValue
// 					}
// 				}
// 			}

// 			if reflect.DeepEqual(podMatchedLables, mappedStoreObj.Deployments[0].Spec.Selector.MatchLabels) && mappedStoreObj.Namespace == podObj.Namespace {
// 				//Found match. Add Pod to this deployment
// 				replaced := false
// 				for _, pod := range mappedStoreObj.Pods {
// 					if pod.UID == podObj.UID {
// 						//UPDATE event
// 						//Its exiting pod object. replace it
// 						pod = *podObj.DeepCopy()
// 						replaced = true
// 						break
// 					}
// 				}

// 				if !replaced {
// 					//Its new pod. Add it to RS
// 					mappedStoreObj.Pods = append(mappedStoreObj.Pods, *podObj.DeepCopy())
// 				}

// 				//Delete and reinsert to ensure Thread Safety
// 				delErr := store.Delete(depItem)
// 				if delErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				addErr := store.Add(mappedStoreObj)
// 				if addErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				*reacted = true
// 			}
// 		}
// 	}
// }

// func getLoneServiceOfPod(podStoreKey, eventType string, store cache.Store, podObj *core_v1.Pod, reacted *bool, errChan chan<- error, wgSvc *sync.WaitGroup) {
// 	//List all service keys
// 	var svcStoreKeys []string
// 	keys := store.ListKeys()
// 	for _, key := range keys {
// 		if strings.Split(key, "/")[1] == "service" {
// 			svcStoreKeys = append(svcStoreKeys, key)
// 		}
// 	}

// 	//Get matching lone ingress for that service and add to service object and delete ingress
// 	wg := sync.WaitGroup{}
// 	for _, svcStoreKey := range svcStoreKeys {
// 		wg.Add(1)
// 		go getMatchingServiceOfPod(svcStoreKey, podStoreKey, eventType, store, podObj.DeepCopy(), reacted, errChan, &wg)
// 	}
// 	wg.Wait()

// 	wgSvc.Done()
// }

// func getMatchingServiceOfPod(svcStoreKey, podStoreKey, eventType string, store cache.Store, podObj *core_v1.Pod, reacted *bool, errChan chan<- error, wg *sync.WaitGroup) {
// 	defer wg.Done()
// 	//Get Lone service Object
// 	svcItem, _, svcErr := store.GetByKey(svcStoreKey)
// 	if svcErr != nil {
// 		errChan <- fmt.Errorf("Error while operating on store")
// 		return
// 	}

// 	switch eventType {
// 	case "ADDED", "UPDATED":
// 		mappedStoreObj := svcItem.(MappedResource)

// 		if mappedStoreObj.Services != nil {
// 			podMatchedLables := make(map[string]string)
// 			for svcKey, svcValue := range mappedStoreObj.Services[0].Spec.Selector {
// 				if val, ok := podObj.Labels[svcKey]; ok {
// 					if val == svcValue {
// 						podMatchedLables[svcKey] = svcValue
// 					}
// 				}
// 			}

// 			if reflect.DeepEqual(podMatchedLables, mappedStoreObj.Services[0].Spec.Selector) && mappedStoreObj.Namespace == podObj.Namespace {
// 				//Found match. Add Pod to this service
// 				replaced := false
// 				for _, pod := range mappedStoreObj.Pods {
// 					if pod.UID == podObj.UID {
// 						//UPDATE event
// 						//Its exiting pod object. replace it
// 						pod = *podObj.DeepCopy()
// 						replaced = true
// 						break
// 					}
// 				}

// 				if !replaced {
// 					//Its new pod. Add it to RS
// 					mappedStoreObj.Pods = append(mappedStoreObj.Pods, *podObj.DeepCopy())
// 				}

// 				//delete and reinsert
// 				delErr := store.Delete(svcItem)
// 				if delErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				addErr := store.Add(mappedStoreObj)
// 				if addErr != nil {
// 					errChan <- fmt.Errorf("Error while operating on store")
// 					return
// 				}

// 				*reacted = true
// 			}
// 		}
// 	}
// }
