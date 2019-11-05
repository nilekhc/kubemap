package kubemap

import (
	"fmt"

	apps_v1 "k8s.io/api/apps/v1"
	apps_v1beta2 "k8s.io/api/apps/v1beta2"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	networking_v1beta1 "k8s.io/api/networking/v1beta1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

//NewK8sMapper maps k8s resources to single common label with ability to provide some options.
//Only one option is supported.
func NewK8sMapper(options *Options) (*Mapper, error) {
	mapper := Mapper{}
	if options != nil {
		if options.Logging != nil {
			zapLogger, zapErr := getZapLogger(options.Logging.LogLevel)
			if zapErr != nil {
				return nil, zapErr
			}
			mapper.log.enabled = options.Logging.Enabled
			mapper.log.logger = zapLogger
		}

		if options.Store != nil {
			mapper.store = options.Store
		}
	}

	if mapper.store == nil {
		mapper.store = cache.NewStore(metaResourceKeyFunc)
	}

	mapper.queue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	return &mapper, nil
}

//MapResources accepts collection different k8s resources of any api versions.
//They will be mapped to respective common label and returned
func (m *Mapper) MapResources(resources K8sResources) (MappedResources, error) {
	addK8sResourcesForMapping(resources, m.queue)

	mappedResources := m.runK8sMap(m.queue, m.store)

	return mappedResources, nil
}

//RunMap starts mapper controller
func (m *Mapper) runK8sMap(queue workqueue.RateLimitingInterface, store cache.Store) MappedResources {
	defer utilruntime.HandleCrash()
	defer queue.ShutDown()

	m.runK8sMapWorker(queue, store)

	return getAllMappedResources(store)
}

func (m *Mapper) runK8sMapWorker(queue workqueue.RateLimitingInterface, store cache.Store) {
	for { // Process until there are no messages in queue.
		if queue.Len() > 0 {
			m.processNextK8sItemToMap(queue, store)
		} else {
			break
		}
	}
}

func (m *Mapper) processNextK8sItemToMap(queue workqueue.RateLimitingInterface, store cache.Store) bool {
	obj, quit := queue.Get()
	if quit {
		return false
	}
	defer queue.Done(obj)
	err := m.processItem(obj, store)
	if err == nil {
		// No error, reset the ratelimit counters
		queue.Forget(obj)
	} else {
		// err != nil and too many retries
		queue.Forget(obj)
		utilruntime.HandleError(err)

		m.warn(fmt.Sprintf("\nError while mapping. Forgetting message from queue.\n"))
	}

	return true
}

func (m *Mapper) processItem(obj interface{}, store cache.Store) error {
	_, err := m.kubemapper(obj, store)
	if err != nil {
		m.error(fmt.Sprintf("\nCannot map resources - %v\n", err))
		return err
	}

	return nil
}

//addK8sResourcesForMapping adds k8s resources of any api versions into the queue for mapping.
func addK8sResourcesForMapping(resources K8sResources, queue workqueue.RateLimitingInterface) {
	//Add ingresses
	for _, ingress := range resources.Ingresses {
		switch object := ingress.(type) {
		case *ext_v1beta1.Ingress:
			queue.Add(gerResourceEvent(object.DeepCopy(), "ingress"))
		case *networking_v1beta1.Ingress:
			queue.Add(gerResourceEvent(object.DeepCopy(), "ingress"))
		}
	}

	//Add services
	for _, service := range resources.Services {
		switch object := service.(type) {
		case *core_v1.Service:
			queue.Add(gerResourceEvent(object.DeepCopy(), "service"))
		}
	}

	//Add deployments
	for _, deployment := range resources.Deployments {
		switch object := deployment.(type) {
		case *apps_v1beta2.Deployment:
			queue.Add(gerResourceEvent(object.DeepCopy(), "deployment"))
		case *apps_v1.Deployment:
			queue.Add(gerResourceEvent(object.DeepCopy(), "deployment"))
		}
	}

	//Add replica sets
	for _, replicaSet := range resources.ReplicaSets {
		switch object := replicaSet.(type) {
		case *ext_v1beta1.ReplicaSet:
			queue.Add(gerResourceEvent(object.DeepCopy(), "replicaset"))
		case *apps_v1.ReplicaSet:
			queue.Add(gerResourceEvent(object.DeepCopy(), "replicaset"))
		}
	}

	//Add pods
	for _, pod := range resources.Pods {
		switch object := pod.(type) {
		case *core_v1.Pod:
			queue.Add(gerResourceEvent(object.DeepCopy(), "pod"))
		}
	}
}
