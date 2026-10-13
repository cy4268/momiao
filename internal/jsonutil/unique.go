// Package jsonutil contains the shared JSON boundary check.
package jsonutil

import "encoding/json"

// Unique rejects repeated object names and values deeper than 12 levels.
func Unique(d *json.Decoder, depth int) bool {
	if depth > 12 {
		return false
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return true
	}
	if delimiter == '{' {
		seen := map[string]bool{}
		for d.More() {
			token, err = d.Token()
			key, ok := token.(string)
			if err != nil || !ok || seen[key] {
				return false
			}
			seen[key] = true
			if !Unique(d, depth+1) {
				return false
			}
		}
		token, err = d.Token()
		return err == nil && token == json.Delim('}')
	}
	if delimiter == '[' {
		for d.More() {
			if !Unique(d, depth+1) {
				return false
			}
		}
		token, err = d.Token()
		return err == nil && token == json.Delim(']')
	}
	return false
}
