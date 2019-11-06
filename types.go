package kubemap

import (
	"go.uber.org/zap"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	network_v1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

//KubeResources is collection of different types of k8s resource for mapping.
//ToDo : Add support for other k8s resources.
type KubeResources struct {
	Ingresses   []network_v1beta1.Ingress
	Services    []core_v1.Service
	Deployments []apps_v1.Deployment
	ReplicaSets []apps_v1.ReplicaSet
	Pods        []core_v1.Pod
}

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
	Ingresses   []network_v1beta1.Ingress `json:"ingresses,omitempty"`
	Services    []core_v1.Service         `json:"services,omitempty"`
	Deployments []apps_v1.Deployment      `json:"deployments,omitempty"`
	ReplicaSets []apps_v1.ReplicaSet      `json:"replicaSets,omitempty"`
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
	log   Logger
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

//MetaIdentifier ...
type MetaIdentifier struct {
	IngressIdentifier     IngressSet `json:"ingressIdentifier,omitempty"`
	ServicesIdentifier    MetaSet    `json:"servicesIdentifier,omitempty"`
	DeploymentsIdentifier MetaSet    `json:"deploymentsIdentifier,omitempty"`
	ReplicaSetsIdentifier []ChildSet `json:"replicaSetsIdentifier,omitempty"`
	PodsIdentifier        []ChildSet `json:"podsIdentifier,omitempty"`
}

//IngressSet ...
type IngressSet struct {
	Names                  []string `json:"names,omitempty"`
	IngressBackendServices []string `json:"ingressBackendServices,omitempty"`
}

//MetaSet ...
type MetaSet struct {
	Names       []string            `json:"names,omitempty"`
	MatchLabels []map[string]string `json:"matchLabels,omitempty"`
}

//ChildSet ...
type ChildSet struct {
	Name            string            `json:"name,omitempty"`
	OwnerReferences []string          `json:"ownerReferences,omitempty"`
	MatchLabels     map[string]string `json:"matchLabels,omitempty"`
}

//MapOptions allows to instantiate new Mapper with custom options
type MapOptions struct {
	Logging LoggingOptions
}

//LoggingOptions ...
type LoggingOptions struct {
	Enabled bool
	//LogLevel sets type of logs viz 'info' or 'debug'.
	LogLevel string
}

//Logger ...
type Logger struct {
	enabled bool
	logger  *zap.SugaredLogger
}
