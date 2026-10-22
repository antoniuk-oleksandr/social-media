package httpjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

var ErrPayloadTooLarge = errors.New("request body too large")

func Decode(r *http.Request, v any) error {
	b, e := io.ReadAll(io.LimitReader(r.Body, 64<<10+1))
	if e != nil {
		return e
	}
	if len(b) > 64<<10 {
		return ErrPayloadTooLarge
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if e = scanValue(d, true); e != nil {
		return e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return fmt.Errorf("trailing value")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
func scanValue(d *json.Decoder, root bool) error {
	t, e := d.Token()
	if e != nil {
		return e
	}
	switch x := t.(type) {
	case json.Delim:
		if root && x != '{' {
			return fmt.Errorf("JSON body must be an object")
		}
		if x == '{' {
			if !root && x != '{' {
				return fmt.Errorf("invalid object")
			}
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return fmt.Errorf("duplicate or invalid key")
				}
				seen[key] = true
				if e = scanValue(d, false); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		}
		if x == '[' {
			for d.More() {
				if e := scanValue(d, false); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		}
		return fmt.Errorf("invalid delimiter")
	}
	if root {
		return fmt.Errorf("JSON body must be an object")
	}
	return nil
}
