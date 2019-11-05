package kubemap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/client-go/util/workqueue"
)

func TestAddResourcesForMapping(t *testing.T) {
	kubeResources := getLegacyTestResources()
	assert.NotNil(t, kubeResources)

	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	addResourcesForMapping(kubeResources, queue)

	t.Logf("Queue Msg count is - %d\n", queue.Len())
	assert.NotZero(t, queue.Len())
}

func TestNewMapper(t *testing.T) {
	kubeResources := getLegacyTestResources()

	mapper := NewMapper()
	assert.NotNil(t, mapper)

	mappedResources, _ := mapper.Map(kubeResources)
	assert.NotNil(t, mappedResources)
}

func TestNewMapperWithOptions(t *testing.T) {
	kubeResources := getLegacyTestResources()

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
