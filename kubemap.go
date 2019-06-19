package kubemap

import (
	"log"

	"k8s.io/client-go/tools/cache"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/util/workqueue"
)

const maxRetries = 5

//NewMapper creates a Mapper to map interlinked K8s resources
func NewMapper() *Mapper {
	store := cache.NewStore(metaResourceKeyFunc)
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	return &Mapper{
		store: store,
		queue: queue,
	}
}

//NewStoreMapper created a mapper that works with existing store.
func NewStoreMapper(store cache.Store) *Mapper {
	return &Mapper{
		store: store,
	}
}

//StoreMap gets a resources and maps it with exiting resources in store
func (m *Mapper) StoreMap(obj interface{}) ([]MapResult, error) {
	mapResults, err := kubemap(obj, m.store)
	if err != nil {
		return []MapResult{}, err
	}

	return mapResults, nil
}

//Map accepts collection different k8s resources.
//They will be mapped to respective common label and returned
func (m *Mapper) Map(resources KubeResources) (MappedResources, error) {
	addResourcesForMapping(resources, m.queue)

	mappedResources := runMap(m.queue, m.store)

	return mappedResources, nil
}

//RunMap starts mapper controller
func runMap(queue workqueue.RateLimitingInterface, store cache.Store) MappedResources {
	defer utilruntime.HandleCrash()
	defer queue.ShutDown()

	runMapWorker(queue, store)

	return getAllMappedResources(store)
}

func runMapWorker(queue workqueue.RateLimitingInterface, store cache.Store) {
	for { // Process until there are no messages in queue.
		if queue.Len() > 0 {
			processNextItemToMap(queue, store)
		} else {
			break
		}
	}
}

func processNextItemToMap(queue workqueue.RateLimitingInterface, store cache.Store) bool {
	obj, quit := queue.Get()
	if quit {
		return false
	}
	defer queue.Done(obj)
	err := processK8sItem(obj, store)
	if err == nil {
		// No error, reset the ratelimit counters
		queue.Forget(obj)
	} else if queue.NumRequeues(obj) < maxRetries {
		queue.AddRateLimited(obj)
	} else {
		// err != nil and too many retries
		queue.Forget(obj)
		utilruntime.HandleError(err)
	}

	return true
}

func processK8sItem(obj interface{}, store cache.Store) error {
	//mapResource(obj, store)
	_, err := kubemap(obj, store)
	if err != nil {
		return err
	}

	return nil
}

func getAllMappedResources(store cache.Store) MappedResources {
	var mappedResources MappedResources
	keys := store.ListKeys()
	for _, key := range keys {
		item, _, _ := store.GetByKey(key)
		mappedResource := item.(MappedResource)
		mappedResources.MappedResource = append(mappedResources.MappedResource, mappedResource)
	}

	return mappedResources
}

func addResourcesForMapping(resources KubeResources, queue workqueue.RateLimitingInterface) {
	//Add ingresses
	for _, ingress := range resources.Ingresses {
		queue.Add(gerResourceEvent(&ingress, "ingress"))
	}

	//Add services
	for _, service := range resources.Services {
		queue.Add(gerResourceEvent(&service, "service"))
	}

	//Add deployments
	for _, deployment := range resources.Deployments {
		queue.Add(gerResourceEvent(&deployment, "deployment"))
	}

	//Add replica sets
	for _, replicaSet := range resources.ReplicaSets {
		queue.Add(gerResourceEvent(&replicaSet, "replicaset"))
	}

	//Add pods
	for _, pod := range resources.Pods {
		queue.Add(gerResourceEvent(&pod, "pod"))
	}
}

func gerResourceEvent(obj interface{}, resourceType string) ResourceEvent {
	var newResourceEvent ResourceEvent
	var err error

	objMeta := objectMetaData(obj)
	newResourceEvent.UID = string(objMeta.UID)
	newResourceEvent.Key, err = cache.MetaNamespaceKeyFunc(obj)
	newResourceEvent.EventType = "ADDED"
	newResourceEvent.ResourceType = resourceType
	newResourceEvent.Namespace = objMeta.Namespace
	newResourceEvent.Name = objMeta.Name
	newResourceEvent.Event = obj
	//newResourceEvent.RawObj = obj

	if err != nil {
		log.Fatalf("Can't get key for store")
	}

	return newResourceEvent
}
