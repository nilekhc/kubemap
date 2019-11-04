package kubemap

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetZapLogger(t *testing.T) {
	getZapLoggerTests := map[string]struct {
		logLevel string
	}{
		"With_DEBUG_Log_Level": {
			logLevel: "debug",
		},
		"With_INFO_Log_Level": {
			logLevel: "info",
		},
	}

	for testName, test := range getZapLoggerTests {
		t.Run(testName, func(t *testing.T) {
			logger, logErr := getZapLogger(test.logLevel)

			assert.NotNil(t, logger)
			assert.Nil(t, logErr)
		})
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
