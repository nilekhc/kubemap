package kubemap

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	apps_v1beta2 "k8s.io/api/apps/v1beta2"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
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

func helperGetK8sResources() KubeResources {
	var kubeResources KubeResources

	//Get Ingress
	var ingress ext_v1beta1.Ingress
	ingressContent := helperGetFileContent("ingress.json")
	json.Unmarshal(ingressContent, &ingress)
	kubeResources.ingresses = append(kubeResources.ingresses, ingress)

	//Get Service
	var service core_v1.Service
	serviceContent := helperGetFileContent("service.json")
	json.Unmarshal(serviceContent, &service)
	kubeResources.services = append(kubeResources.services, service)

	//Get Deployment
	var deployment apps_v1beta2.Deployment
	deploymentContent := helperGetFileContent("deployment.json")
	json.Unmarshal(deploymentContent, &deployment)
	kubeResources.deployments = append(kubeResources.deployments, deployment)

	//Get Replica Set
	var replicaSet ext_v1beta1.ReplicaSet
	replicaSetContent := helperGetFileContent("replicaset.json")
	json.Unmarshal(replicaSetContent, &replicaSet)
	kubeResources.replicaSets = append(kubeResources.replicaSets, replicaSet)

	//Get pod
	var pod core_v1.Pod
	podContent := helperGetFileContent("pod.json")
	json.Unmarshal(podContent, &pod)
	kubeResources.pods = append(kubeResources.pods, pod)

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
