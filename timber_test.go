package timber

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConsole(t *testing.T) {
	log := NewTimber()
	console := new(ConsoleWriter)
	formatter := NewPatFormatter("[%D %T] [%L] %-10x %M")
	idx := log.AddLogger(ConfigLogger{LogWriter: console,
		Level:     DEBUG,
		Formatter: formatter})
	log.Error("what error? %v", idx)
	log.Close()
}

func TestFile(t *testing.T) {
	log := NewTimber()
	writer, _ := NewFileWriter("test.log")
	formatter := NewPatFormatter("[%D %T] [%L] %-10x %M")
	idx := log.AddLogger(ConfigLogger{LogWriter: writer,
		Level:     FINEST,
		Formatter: formatter})
	log.Error("what error? %v", idx)
	log.Warn("I'm waringing you!")
	log.Info("FYI")
	log.Fine("you soo fine!")
	log.Close()
}

func TestXmlConfig(t *testing.T) {
	log := NewTimber()
	log.LoadXMLConfig("timber.xml")
	log.Info("Message to XML loggers")
	log.Close()
}

func TestJsonConfig(t *testing.T) {
	log := NewTimber()
	log.LoadJSONConfig("timber.json")
	log.Info("Message to JSON loggers")
	log.Close()
}

func TestDefaultLogger(t *testing.T) {
	console := new(ConsoleWriter)
	formatter := NewPatFormatter("%DT%T %L %-10x %M")
	AddLogger(ConfigLogger{LogWriter: console,
		Level:     DEBUG,
		Formatter: formatter})
	Warn("Some sweet default logging")
	Close()
}

func TestDoubleClose(t *testing.T) {
	log := NewTimber()
	console := new(ConsoleWriter)
	formatter := NewPatFormatter("%DT%T %L %-10x %M")
	log.AddLogger(ConfigLogger{LogWriter: console,
		Level:     DEBUG,
		Formatter: formatter})
	log.Close()
	log.Close() // call Close twice
	log.Warn("Don't panic")
}

// writer to capture logs for testing purposes
type TestWriter struct {
	logs []string
}

func (tw *TestWriter) LogWrite(msg string) {
	tw.logs = append(tw.logs, msg)
}

func (tw *TestWriter) Close() {
	// Nothing
}

func TestJSONFormatterLogger(t *testing.T) {
	a := assert.New(t)

	log := NewTimber()
	testWriter := new(TestWriter)
	formatter := NewJSONFormatter()
	log.AddLogger(
		ConfigLogger{
			LogWriter: testWriter,
			Level:     DEBUG,
			Formatter: formatter,
		},
	)
	log.Info("Some JSON logging")
	log.InfoEx(
		map[string]string{
			"testExtra":        "hello",
			"testAnotherExtra": "goodbye",
		},
		"Some JSON logging with some extra fields",
	)
	log.Close()

	firstLog := testWriter.logs[0]
	secondLog := testWriter.logs[1]

	var firstLogMap map[string]interface{}
	var secondLogMap map[string]interface{}

	json.Unmarshal([]byte(firstLog), &firstLogMap)
	json.Unmarshal([]byte(secondLog), &secondLogMap)

	expectedSharedKeys := []string{"Level", "timestamp", "SourceFile", "SourceFile", "message", "FuncPath", "FuncPath", "PackagePath", "HostName"}

	// just check for presence of keys, as things like timestamp, hostname etc dependent on environment
	for _, k := range expectedSharedKeys {
		_, ok := firstLogMap[k]
		a.True(ok)
		_, ok = secondLogMap[k]
		a.True(ok)
	}

	// first log should have expected message, and no "extra"
	a.Equal(firstLogMap["message"], "Some JSON logging")
	_, ok := firstLogMap["extra"]
	a.False(ok)

	// second log should have message, and "extra" fields
	a.Equal(secondLogMap["message"], "Some JSON logging with some extra fields")
	extra, ok := secondLogMap["extra"]
	a.True(ok)
	mapExtra := extra.(map[string]interface{})
	a.Equal(mapExtra["testExtra"], "hello")
	a.Equal(mapExtra["testAnotherExtra"], "goodbye")
}
