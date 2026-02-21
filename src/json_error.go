package app

import (
	"fmt"
	"os"
)

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

func writeJSONError(errorCode, message string, details map[string]any) {
	payload := map[string]any{
		"ok":        false,
		"errorCode": errorCode,
		"error":     message,
	}
	for k, v := range details {
		payload[k] = v
	}
	writeJSON(payload)
}

func commandError(jsonOut bool, errorCode, message string) int {
	if jsonOut {
		writeJSONError(errorCode, message, nil)
		return 1
	}
	fmt.Fprintln(os.Stderr, message)
	return 1
}

func commandErrorf(jsonOut bool, errorCode, format string, args ...any) int {
	return commandError(jsonOut, errorCode, fmt.Sprintf(format, args...))
}
