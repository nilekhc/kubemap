package kubemap

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	apps_v1 "k8s.io/api/apps/v1"
	apps_v1beta2 "k8s.io/api/apps/v1beta2"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	networking_v1beta1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeClient "k8s.io/client-go/kubernetes/fake"
)

var (
	testClient *fakeClient.Clientset
)

func TestMain(m *testing.M) {
	setup()
	os.Exit(m.Run())
}

func setup() {
	testClient = fakeClient.NewSimpleClientset()
	helperGetK8sResources(testClient)
}

func helperGetK8sResources(testClient *fakeClient.Clientset) {
	//Create fake NS
	var ns core_v1.Namespace
	nsContent := helperGetFileContent(filepath.Join("namespace.json"))
	json.Unmarshal(nsContent, &ns)
	_, err := testClient.CoreV1().Namespaces().Create(&ns)
	if err != nil {
		panic(err)
	}

	//Get fixture 1
	//Get Ingress
	var ingress1 ext_v1beta1.Ingress
	ingressContent := helperGetFileContent(filepath.Join("1", "ingress.json"))
	json.Unmarshal(ingressContent, &ingress1)
	_, err = testClient.ExtensionsV1beta1().Ingresses(ns.Name).Create(&ingress1)
	if err != nil {
		panic(err)
	}

	//Get Service
	var service1 core_v1.Service
	serviceContent := helperGetFileContent(filepath.Join("1", "service.json"))
	json.Unmarshal(serviceContent, &service1)
	_, err = testClient.CoreV1().Services(ns.Name).Create(&service1)
	if err != nil {
		panic(err)
	}

	//Get Deployment
	var deployment1 apps_v1beta2.Deployment
	deploymentContent := helperGetFileContent(filepath.Join("1", "deployment.json"))
	json.Unmarshal(deploymentContent, &deployment1)
	_, err = testClient.AppsV1beta2().Deployments(ns.Name).Create(&deployment1)
	if err != nil {
		panic(err)
	}

	//Get Replica Set
	var replicaSet1 ext_v1beta1.ReplicaSet
	replicaSetContent := helperGetFileContent(filepath.Join("1", "replicaset.json"))
	json.Unmarshal(replicaSetContent, &replicaSet1)
	_, err = testClient.ExtensionsV1beta1().ReplicaSets(ns.Name).Create(&replicaSet1)
	if err != nil {
		panic(err)
	}

	//Get pod
	var pod1 core_v1.Pod
	podContent := helperGetFileContent(filepath.Join("1", "pod.json"))
	json.Unmarshal(podContent, &pod1)
	_, err = testClient.CoreV1().Pods(ns.Name).Create(&pod1)
	if err != nil {
		panic(err)
	}

	//Get fixture 2
	//Get Ingress
	var ingress2 networking_v1beta1.Ingress
	ingressContent2 := helperGetFileContent(filepath.Join("2", "ingress.json"))
	json.Unmarshal(ingressContent2, &ingress2)
	_, err = testClient.NetworkingV1beta1().Ingresses(ns.Name).Create(&ingress2)
	if err != nil {
		panic(err)
	}

	//Get Service
	var service2 core_v1.Service
	serviceContent2 := helperGetFileContent(filepath.Join("2", "service.json"))
	json.Unmarshal(serviceContent2, &service2)
	_, err = testClient.CoreV1().Services(ns.Name).Create(&service2)
	if err != nil {
		panic(err)
	}

	//Get Deployment
	var deployment2 apps_v1.Deployment
	deploymentContent2 := helperGetFileContent(filepath.Join("2", "deployment.json"))
	json.Unmarshal(deploymentContent2, &deployment2)
	_, err = testClient.AppsV1().Deployments(ns.Name).Create(&deployment2)
	if err != nil {
		panic(err)
	}

	//Get Replica Set
	var replicaSet2 apps_v1.ReplicaSet
	replicaSetContent2 := helperGetFileContent(filepath.Join("2", "replicaset.json"))
	json.Unmarshal(replicaSetContent2, &replicaSet2)
	_, err = testClient.AppsV1().ReplicaSets(ns.Name).Create(&replicaSet2)
	if err != nil {
		panic(err)
	}

	//Get pod
	var pod2 core_v1.Pod
	podContent2 := helperGetFileContent(filepath.Join("2", "pod.json"))
	json.Unmarshal(podContent2, &pod2)
	_, err = testClient.CoreV1().Pods(ns.Name).Create(&pod2)
	if err != nil {
		panic(err)
	}
}

func helperGetFileContent(fileName string) []byte {
	path := filepath.Join("testdata", "test-fixtures", fileName)

	content, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}

	return content
}

func getLegacyTestResources() KubeResources {
	kubeResources := KubeResources{}

	ingress, _ := testClient.ExtensionsV1beta1().Ingresses("test-namespace").Get("kube-map-ingress-1", metav1.GetOptions{})
	kubeResources.Ingresses = append(kubeResources.Ingresses, *ingress.DeepCopy())

	service, _ := testClient.CoreV1().Services("test-namespace").Get("kube-map-service-1", metav1.GetOptions{})
	kubeResources.Services = append(kubeResources.Services, *service.DeepCopy())

	deployment, _ := testClient.AppsV1beta2().Deployments("test-namespace").Get("kube-map-deployment-1", metav1.GetOptions{})
	kubeResources.Deployments = append(kubeResources.Deployments, *deployment.DeepCopy())

	replicaSet, _ := testClient.ExtensionsV1beta1().ReplicaSets("test-namespace").Get("kube-map-replicaset-1-644c5c58fc", metav1.GetOptions{})
	kubeResources.ReplicaSets = append(kubeResources.ReplicaSets, *replicaSet.DeepCopy())

	pod, _ := testClient.CoreV1().Pods("test-namespace").Get("kube-map-pod-1-644c5c58fc-ggdmn", metav1.GetOptions{})
	kubeResources.Pods = append(kubeResources.Pods, *pod.DeepCopy())

	return kubeResources
}

func getTestResourcesByExample(example int) K8sResources {
	kubeResources := K8sResources{}

	ingress, _ := testClient.NetworkingV1beta1().Ingresses("test-namespace").Get(fmt.Sprintf("kube-map-ingress-%d", example), metav1.GetOptions{})
	kubeResources.Ingresses = append(kubeResources.Ingresses, ingress.DeepCopy())

	service, _ := testClient.CoreV1().Services("test-namespace").Get(fmt.Sprintf("kube-map-service-%d", example), metav1.GetOptions{})
	kubeResources.Services = append(kubeResources.Services, service.DeepCopy())

	deployment, _ := testClient.AppsV1().Deployments("test-namespace").Get(fmt.Sprintf("kube-map-deployment-%d", example), metav1.GetOptions{})
	kubeResources.Deployments = append(kubeResources.Deployments, deployment.DeepCopy())

	replicaSet, _ := testClient.AppsV1().ReplicaSets("test-namespace").Get(fmt.Sprintf("kube-map-replicaset-%d-644c5c58fc", example), metav1.GetOptions{})
	kubeResources.ReplicaSets = append(kubeResources.ReplicaSets, replicaSet.DeepCopy())

	pod, _ := testClient.CoreV1().Pods("test-namespace").Get(fmt.Sprintf("kube-map-pod-%d-644c5c58fc-ggdmn", example), metav1.GetOptions{})
	kubeResources.Pods = append(kubeResources.Pods, pod.DeepCopy())

	return kubeResources
}
