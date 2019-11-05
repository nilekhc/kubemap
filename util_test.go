package kubemap

import (
	"testing"

	"github.com/magiconair/properties/assert"
)

func TestCopyMappedResource(t *testing.T) {
	kubeResources := helperGetK8sResources()

	mapper, _ := NewMapperWithOptions(MapOptions{
		Logging: LoggingOptions{
			Enabled:  false,
			LogLevel: "debug",
		},
	})

	mappedResources, _ := mapper.Map(kubeResources)

	for _, mappedResource := range mappedResources.MappedResource {
		copyOfMappedResource := copyMappedResource(mappedResource)

		assert.Equal(t, mappedResource.Kube.Ingresses[0].Name, copyOfMappedResource.Kube.Ingresses[0].Name)
	}
}
