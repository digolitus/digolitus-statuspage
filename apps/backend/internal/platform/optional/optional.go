// Package optional distinguishes, in JSON PATCH payloads, a field that is
// absent (leave unchanged) from one explicitly set to null (clear it).
package optional

import "encoding/json"

// Nullable is a tri-state JSON value: absent (Set=false), null (Set=true,
// Value=nil) or a value (Set=true, Value!=nil).
type Nullable[T any] struct {
	Set   bool
	Value *T
}

// UnmarshalJSON is only invoked when the key is present in the payload.
func (n *Nullable[T]) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}
