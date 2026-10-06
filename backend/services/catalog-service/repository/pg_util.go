package repository

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// errRecordNotFound keeps the text callers match on (strings.Contains "not found").
var errRecordNotFound = errors.New("record not found")

// jsonArray renders a slice for a jsonb array column ("[]" for nil).
func jsonArray(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return "[]"
	}
	return string(b)
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}
