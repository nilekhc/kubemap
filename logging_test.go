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
		"With_NonSupported_Log_Level": {
			logLevel: "NonSupported",
		},
	}

	for testName, test := range getZapLoggerTests {
		t.Run(testName, func(t *testing.T) {
			logger, logErr := getZapLogger(test.logLevel)
			if test.logLevel != "NonSupported" {
				assert.NotNil(t, logger)
				assert.Nil(t, logErr)
			} else {
				assert.Nil(t, logger)
				assert.NotNil(t, logErr)
			}
		})
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
