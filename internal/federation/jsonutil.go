package federation

import (
	"encoding/json"
	"net/url"
	"strings"
)

func typeOf(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		if len(t) > 0 {
			if s, ok := t[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

func asString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func objectID(v any) string {
	if s := asString(v); s != "" {
		return s
	}
	if m := asMap(v); m != nil {
		return asString(m["id"])
	}
	return ""
}

func actorIRI(v any) string {
	if s := asString(v); s != "" {
		return s
	}
	if m := asMap(v); m != nil {
		return asString(m["id"])
	}
	return ""
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func decodeMap(body []byte) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func encodeJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

func sameIRI(a, b string) bool {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(a)), "/") ==
		strings.TrimSuffix(strings.ToLower(strings.TrimSpace(b)), "/")
}

func actorFromKeyID(keyID string) string {
	if i := strings.Index(keyID, "#"); i >= 0 {
		return keyID[:i]
	}
	return keyID
}
