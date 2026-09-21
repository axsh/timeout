package timeout

import "encoding/json"

// MarshalDetailsJSON marshals details for JSON output.
// On failure it returns (nil, false); callers must omit the details key.
func MarshalDetailsJSON(details any) (json.RawMessage, bool) {
	if details == nil {
		return nil, true
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return nil, false
	}
	return raw, true
}
