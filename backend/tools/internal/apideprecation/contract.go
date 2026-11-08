// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package apideprecation reads release notices from the public contract.
package apideprecation

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"

	policy "github.com/margince/margince/backend/internal/platform/apideprecation"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

func Notices(doc *openapi3.T) (map[string]policy.Notice, error) {
	notices := map[string]policy.Notice{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			raw, marked := op.Extensions[policy.NoticeExtension]
			if marked && !op.Deprecated {
				return nil, fmt.Errorf("%s %s has a notice but is not deprecated", method, path)
			}
			if !op.Deprecated {
				continue
			}
			var notice policy.Notice
			if marked {
				encoded, err := json.Marshal(raw)
				if err != nil {
					return nil, err
				}
				decoder := json.NewDecoder(bytes.NewReader(encoded))
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&notice); err != nil {
					return nil, fmt.Errorf("%s %s: %w", method, path, err)
				}
			}
			if err := notice.Validate(); err != nil {
				return nil, fmt.Errorf("%s %s: %w", method, path, err)
			}
			notices[method+" "+httpserver.BaseURL+path] = notice
		}
	}
	return notices, nil
}

// References need not resolve to read operation notices. The caller already
// flattened YAML aliases, and unrelated unreachable schemas may be incomplete.
func FromYAML(root *yaml.Node) (map[string]policy.Notice, error) {
	var raw map[string]any
	if err := root.Decode(&raw); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var doc openapi3.T
	if err := json.Unmarshal(encoded, &doc); err != nil {
		return nil, err
	}
	return Notices(&doc)
}
