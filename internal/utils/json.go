package utils

import (
	"encoding/json"
)

func SafeJsonParse(text string, v interface{}) error {
	return json.Unmarshal([]byte(text), v)
}

func JsonStringifySafe(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}
