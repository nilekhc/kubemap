package kubemap

import (
	apps_v1beta2 "k8s.io/api/apps/v1beta2"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

//KubeResources is collection of different types of k8s resource for mapping.
//ToDo : Add support for other k8s resources.
type KubeResources struct {
	ingresses   []ext_v1beta1.Ingress
	services    []core_v1.Service
	deployments []apps_v1beta2.Deployment
	replicaSets []ext_v1beta1.ReplicaSet
	pods        []core_v1.Pod
}

// //MappedResource is final mapped output of interlinked K8s resources
// type MappedResource struct {
// 	CommonLabel string                    `json:"commonLabel,omitempty"`
// 	Namespace   string                    `json:"namespace,omitempty"`
// 	CurrentType string                    `json:"currentType,omitempty"`
// 	Ingresses   []ext_v1beta1.Ingress     `json:"ingresses,omitempty"`
// 	Services    []core_v1.Service         `json:"services,omitempty"`
// 	Deployments []apps_v1beta2.Deployment `json:"deployments,omitempty"`
// 	ReplicaSets []ext_v1beta1.ReplicaSet  `json:"replicaSets,omitempty"`
// 	Pods        []core_v1.Pod             `json:"pods,omitempty"`
// }

//MappedResource is final mapped output of interlinked K8s resources
type MappedResource struct {
	CommonLabel string `json:"commonLabel,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	CurrentType string `json:"currentType,omitempty"`
	EventType   string `json:"eventType,omitempty"`
	Kube        Kube   `json:"kube,omitempty"`
}

//Kube ...
type Kube struct {
	Ingresses   []ext_v1beta1.Ingress     `json:"ingresses,omitempty"`
	Services    []core_v1.Service         `json:"services,omitempty"`
	Deployments []apps_v1beta2.Deployment `json:"deployments,omitempty"`
	ReplicaSets []ext_v1beta1.ReplicaSet  `json:"replicaSets,omitempty"`
	Pods        []core_v1.Pod             `json:"pods,omitempty"`
	Events      []core_v1.Event           `json:"events,omitempty"`
}

//MappedResources returns set of common labels consisting mapped k8s resources.
type MappedResources struct {
	MappedResource []MappedResource `json:"mappedResource,omitempty"`
}

// Mapper hold internal store and workqueue for mapping
type Mapper struct {
	queue workqueue.RateLimitingInterface
	store cache.Store
}

//ResourceEvent ...
type ResourceEvent struct {
	UID          string
	Key          string
	EventType    string
	Namespace    string
	ResourceType string
	Name         string
	Event        interface{}
	// RawObj        interface{}
	// UpdatedRawObj interface{}
}

//MapResult ...
type MapResult struct {
	Key            string
	Action         string
	Message        string
	CommonLabel    string
	DeleteKeys     []string
	IsMapped       bool
	IsStoreUpdated bool
	MappedResource MappedResource
}
