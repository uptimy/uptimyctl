package workspace

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The API stores an HTTP check as steps with assertions:
//
//	{"proxyUrl"?, "steps": [{"name", "request": {"method", "uri", "headers"?, "body"?},
//	  "assertions"?, "extract"?, "timeoutSeconds"?}]}
//
// A file still writes an HTTP monitor as a target (and config overrides in
// the flat keys: headers, body, expectedStatusCode, …). httpCanonical turns
// either shape into the one the API stores (upti.my-api
// src/utils/http_check_config.ts, normalizeHttpCheckConfig), so a file and
// a live monitor compare equal when the API would store the same check.

// httpCanonical is an HTTP config in the stored shape.
func httpCanonical(cfg map[string]any) map[string]any {
	out := map[string]any{}
	if p, _ := cfg["proxyUrl"].(string); strings.TrimSpace(p) != "" {
		out["proxyUrl"] = strings.TrimSpace(p)
	}
	var steps []any
	if list, ok := cfg["steps"].([]any); ok && len(list) > 0 {
		for i, s := range list {
			step, _ := s.(map[string]any)
			steps = append(steps, cleanStep(step, i))
		}
	} else {
		steps = []any{cleanStep(flatStep(cfg), 0)}
	}
	out["steps"] = steps
	m, _ := normalized(out).(map[string]any)
	return m
}

// flatStep is the flat config as one step: expectedStatusCode becomes a
// status assertion, expectedResponseBody one json assertion per field.
func flatStep(cfg map[string]any) map[string]any {
	var assertions []any
	if list, ok := cfg["assertions"].([]any); ok {
		assertions = append(assertions, list...)
	}
	hasStatus := false
	for _, a := range assertions {
		if m, ok := a.(map[string]any); ok && m["source"] == "status" {
			hasStatus = true
		}
	}
	if code := intOf(cfg["expectedStatusCode"]); code > 0 && !hasStatus {
		assertions = append([]any{map[string]any{"source": "status", "op": "eq", "value": code}}, assertions...)
	}
	for _, l := range leaves(objectOf(cfg["expectedResponseBody"]), "") {
		assertions = append(assertions, map[string]any{"source": "json", "path": l.path, "op": "eq", "value": l.value})
	}
	step := map[string]any{
		"name": "request",
		"request": map[string]any{
			"method":  cfg["method"],
			"uri":     cfg["uri"],
			"headers": objectOf(cfg["headers"]),
			"body":    cfg["body"],
		},
	}
	if len(assertions) > 0 {
		step["assertions"] = assertions
	}
	if list, ok := cfg["extract"].([]any); ok {
		step["extract"] = list
	}
	return step
}

// cleanStep drops empty fields the way the API does before storing.
func cleanStep(step map[string]any, index int) map[string]any {
	req, _ := step["request"].(map[string]any)
	if req == nil {
		req = map[string]any{}
	}
	method, _ := req["method"].(string)
	if method == "" {
		method = "GET"
	}
	uri, _ := req["uri"].(string)
	request := map[string]any{"method": strings.ToUpper(method), "uri": strings.TrimSpace(uri)}
	if headers := objectOf(req["headers"]); len(headers) > 0 {
		h := map[string]any{}
		for k, v := range headers {
			h[k] = fmt.Sprint(v)
		}
		request["headers"] = h
	}
	if !emptyBody(req["body"]) {
		request["body"] = req["body"]
	}

	name, _ := step["name"].(string)
	if strings.TrimSpace(name) == "" {
		name = "step " + strconv.Itoa(index+1)
	}
	out := map[string]any{"name": strings.TrimSpace(name), "request": request}
	if list, ok := step["assertions"].([]any); ok && len(list) > 0 {
		out["assertions"] = list
	}
	if list, ok := step["extract"].([]any); ok && len(list) > 0 {
		out["extract"] = list
	}
	if t, ok := step["timeoutSeconds"]; ok && t != nil {
		out["timeoutSeconds"] = t
	}
	return out
}

// httpTargetOf reads a canonical config as a target and expected status,
// when it is nothing more than that: one GET with one status eq assertion.
func httpTargetOf(cfg map[string]any) (target string, status int, ok bool) {
	steps, _ := cfg["steps"].([]any)
	if len(steps) != 1 || cfg["proxyUrl"] != nil {
		return "", 0, false
	}
	step, _ := steps[0].(map[string]any)
	req, _ := step["request"].(map[string]any)
	if len(req) != 2 || req["method"] != "GET" || len(step) != 3 {
		return "", 0, false
	}
	assertions, _ := step["assertions"].([]any)
	if len(assertions) != 1 {
		return "", 0, false
	}
	a, _ := assertions[0].(map[string]any)
	if len(a) != 3 || a["source"] != "status" || a["op"] != "eq" {
		return "", 0, false
	}
	target, _ = req["uri"].(string)
	return target, intOf(a["value"]), target != ""
}

func emptyBody(v any) bool {
	switch b := v.(type) {
	case nil:
		return true
	case string:
		t := strings.TrimSpace(b)
		return t == "" || t == "null" || t == "{}"
	case map[string]any:
		return len(b) == 0
	}
	return false
}

// objectOf reads a JSON object given as a map or as a JSON string.
func objectOf(v any) map[string]any {
	switch o := v.(type) {
	case map[string]any:
		return o
	case string:
		var m map[string]any
		if json.Unmarshal([]byte(o), &m) == nil {
			return m
		}
	}
	return nil
}

func intOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	}
	return 0
}

type leaf struct {
	path  string
	value any
}

// leaves flattens {"a": {"b": 1}} to a.b = 1, keys sorted; arrays are leaves.
func leaves(m map[string]any, prefix string) []leaf {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []leaf
	for _, k := range keys {
		if child, ok := m[k].(map[string]any); ok && len(child) > 0 {
			out = append(out, leaves(child, prefix+k+".")...)
			continue
		}
		out = append(out, leaf{prefix + k, m[k]})
	}
	return out
}
