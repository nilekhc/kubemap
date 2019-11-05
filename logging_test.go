package kubemap

import (
	"bytes"
	"io"
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

func TestDebugLogging(t *testing.T) {
	old := os.Stdout // keep backup of the real stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	debugLoggingTests := map[string]struct {
		logLevel   string
		logMessage string
	}{
		"With_Log_Level_Set_To_DEBUG": {
			logLevel:   "debug",
			logMessage: "This is DEBUG log",
		},
		"With_Log_Level_Set_To_INFO": {
			logLevel:   "info",
			logMessage: "This is DEBUG log",
		},
	}

	for testName, test := range debugLoggingTests {
		t.Run(testName, func(t *testing.T) {
			mapper, _ := NewMapperWithOptions(MapOptions{
				Logging: LoggingOptions{
					Enabled:  true,
					LogLevel: test.logLevel,
				},
			})

			mapper.debug(test.logMessage)

			outC := make(chan string)
			// copy the output in a separate goroutine so printing can't block indefinitely
			go func() {
				var buf bytes.Buffer
				io.Copy(&buf, r)
				outC <- buf.String()
			}()

			// back to normal state
			w.Close()
			os.Stdout = old // restoring the real stdout
			out := <-outC

			switch test.logLevel {
			case "debug":
				assert.Contains(t, out, test.logMessage)
			case "info":
				assert.Empty(t, out)
			}
		})
	}
}

func TestHigherLogLevel(t *testing.T) {
	debugLoggingTests := map[string]struct {
		logType    string
		logMessage string
	}{
		"With_Log_Level_INFO": {
			logType:    "info",
			logMessage: "This is INFO log",
		},
		"With_Log_Level_WARN": {
			logType:    "warn",
			logMessage: "This is WARN log",
		},
		"With_Log_Level_ERROR": {
			logType:    "error",
			logMessage: "This is ERROR log",
		},
	}

	for testName, test := range debugLoggingTests {
		t.Run(testName, func(t *testing.T) {
			old := os.Stdout // keep backup of the real stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			mapper, _ := NewMapperWithOptions(MapOptions{
				Logging: LoggingOptions{
					Enabled:  true,
					LogLevel: test.logType,
				},
			})

			switch test.logType {
			case "info":
				mapper.info(test.logMessage)
			case "warn":
				mapper.warn(test.logMessage)
			case "error":
				mapper.error(test.logMessage)
			}

			outC := make(chan string)
			// copy the output in a separate goroutine so printing can't block indefinitely
			go func() {
				var buf bytes.Buffer
				io.Copy(&buf, r)
				outC <- buf.String()
			}()

			// back to normal state
			w.Close()
			os.Stdout = old // restoring the real stdout
			out := <-outC

			assert.Contains(t, out, test.logMessage)
		})
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
