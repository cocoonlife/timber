package timber

import (
	"encoding/json"
	"fmt"
)

type JSONFormatter struct {
}

func NewJSONFormatter() *JSONFormatter {
	return &JSONFormatter{}
}

func (f *JSONFormatter) Format(rec *LogRecord) string {
	if msg, err := json.Marshal(rec); err == nil {
		return string(msg)
	} else {
		return fmt.Sprintf("JSON Marshal Fail:%s - %s", err.Error(), rec)
	}
}
