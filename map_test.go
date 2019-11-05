package kubemap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

func TestAddK8sResourcesForMapping(t *testing.T) {
	resourcesTests := map[string]struct {
		kubeResources K8sResources
		queue         workqueue.RateLimitingInterface
	}{
		"With_Test_Example_2": {
			kubeResources: getTestResourcesByExample(2),
			queue:         workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		},
	}

	for testName, test := range resourcesTests {
		t.Run(testName, func(t *testing.T) {
			addK8sResourcesForMapping(test.kubeResources, test.queue)

			assert.NotNil(t, test.kubeResources)
			assert.NotZero(t, test.queue.Len())
		})
	}
}

func TestNewK8sMapper(t *testing.T) {
	newMapperTests := map[string]struct {
		options      *Options
		isLogEnabled bool
	}{
		"Without_Options": {
			options:      nil,
			isLogEnabled: false,
		},
		"With_Log_Options": {
			options: &Options{
				Logging: &LoggingOptions{
					Enabled:  true,
					LogLevel: "debug",
				},
			},
			isLogEnabled: true,
		},
		"With_Store_Options": {
			options: &Options{
				Store: cache.NewStore(metaResourceKeyFunc),
			},
			isLogEnabled: false,
		},
		"With_Log_And_Store_Options": {
			options: &Options{
				Logging: &LoggingOptions{
					Enabled:  true,
					LogLevel: "debug",
				},
				Store: cache.NewStore(metaResourceKeyFunc),
			},
			isLogEnabled: true,
		},
	}

	for testName, test := range newMapperTests {
		t.Run(testName, func(t *testing.T) {
			mapper, err := NewK8sMapper(test.options)

			assert.Nil(t, err)

			assert.Equal(t, test.isLogEnabled, mapper.log.enabled)
			if test.isLogEnabled {
				assert.NotNil(t, mapper.log.logger)
			} else {
				assert.Nil(t, mapper.log.logger)
			}

			assert.NotNil(t, mapper.queue)
			assert.NotNil(t, mapper.store)
		})
	}
}
