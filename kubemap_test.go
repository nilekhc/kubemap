package kubemap

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	network_v1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/client-go/util/workqueue"
)

func TestAddResourcesForMapping(t *testing.T) {
	kubeResources := helperGetK8sResources()
	assert.NotNil(t, kubeResources)

	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	addResourcesForMapping(kubeResources, queue)

	t.Logf("Queue Msg count is - %d\n", queue.Len())
	assert.NotZero(t, queue.Len())
}

func TestNewMapper(t *testing.T) {
	kubeResources := helperGetK8sResources()

	mapper := NewMapper()
	assert.NotNil(t, mapper)

	mappedResources, _ := mapper.Map(kubeResources)
	assert.NotNil(t, mappedResources)
}

func TestNewMapperWithOptions(t *testing.T) {
	kubeResources := helperGetK8sResources()

	mapper, _ := NewMapperWithOptions(MapOptions{
		Logging: LoggingOptions{
			Enabled:  true,
			LogLevel: "debug",
		},
	})

	assert.NotNil(t, mapper)

	mappedResources, _ := mapper.Map(kubeResources)
	assert.NotNil(t, mappedResources)
}

func helperGetK8sResources() KubeResources {
	var kubeResources KubeResources

	//Get Ingress
	var ingress network_v1beta1.Ingress
	ingressContent := helperGetFileContent("ingress.json")
	json.Unmarshal(ingressContent, &ingress)
	kubeResources.Ingresses = append(kubeResources.Ingresses, ingress)

	//Get Service
	var service core_v1.Service
	serviceContent := helperGetFileContent("service.json")
	json.Unmarshal(serviceContent, &service)
	kubeResources.Services = append(kubeResources.Services, service)

	//Get Deployment
	var deployment apps_v1.Deployment
	deploymentContent := helperGetFileContent("deployment.json")
	json.Unmarshal(deploymentContent, &deployment)
	kubeResources.Deployments = append(kubeResources.Deployments, deployment)

	//Get Replica Set
	var replicaSet apps_v1.ReplicaSet
	replicaSetContent := helperGetFileContent("replicaset.json")
	json.Unmarshal(replicaSetContent, &replicaSet)
	kubeResources.ReplicaSets = append(kubeResources.ReplicaSets, replicaSet)

	//Get pod
	var pod core_v1.Pod
	podContent := helperGetFileContent("pod.json")
	json.Unmarshal(podContent, &pod)
	kubeResources.Pods = append(kubeResources.Pods, pod)

	return kubeResources
}

func helperGetFileContent(fileName string) []byte {
	path := filepath.Join("testdata", "test-fixtures", fileName)

	content, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}

	return content
}
