// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"strconv"
)

// errUnreadable refuses a body this intake cannot read as form fields.
var errUnreadable = errors.New("the body is not form fields; send it urlencoded, as multipart/form-data or as a JSON object")

// parseFields reads a urlencoded, multipart or flat JSON body. A file part of a
// multipart body is skipped: a form's attachments are not a lead's data.
func parseFields(contentType string, body []byte) ([]field, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, errUnreadable
	}
	switch mediaType {
	case "application/x-www-form-urlencoded":
		return urlencodedFields(body)
	case "multipart/form-data":
		return multipartFields(body, params["boundary"])
	case "application/json":
		return jsonFields(body)
	}
	return nil, errUnreadable
}

func urlencodedFields(body []byte) ([]field, error) {
	var out []field
	for len(body) > 0 && len(out) < maxFields {
		var pair []byte
		pair, body, _ = bytes.Cut(body, []byte("&"))
		name, value, _ := bytes.Cut(pair, []byte("="))
		n, err := url.QueryUnescape(string(name))
		if err != nil {
			return nil, errUnreadable
		}
		v, err := url.QueryUnescape(string(value))
		if err != nil {
			return nil, errUnreadable
		}
		if n != "" {
			out = append(out, field{Name: n, Value: v})
		}
	}
	return out, nil
}

func multipartFields(body []byte, boundary string) ([]field, error) {
	if boundary == "" {
		return nil, errUnreadable
	}
	// upload:route /v1/public/forms/{form_key}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var out []field
	for len(out) < maxFields {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, errUnreadable
		}
		if part.FileName() != "" || part.FormName() == "" {
			continue
		}
		value, err := io.ReadAll(io.LimitReader(part, maxMessageChars*4))
		if err != nil {
			return nil, errUnreadable
		}
		out = append(out, field{Name: part.FormName(), Value: string(value)})
	}
	return out, nil
}

// jsonFields reads one flat JSON object. A nested value is kept as its JSON
// text, so a sender's structure still reaches the timeline.
func jsonFields(body []byte) ([]field, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return nil, errUnreadable
	}
	values := make(map[string]string, len(object))
	for name, raw := range object {
		values[name] = scalarText(raw)
	}
	return sortedFields(values), nil
}

// scalarText renders one JSON value as the text a visitor typed: a string
// unquoted, a number or boolean as written, anything else as its JSON.
func scalarText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return strconv.FormatBool(b)
	}
	if string(raw) == "null" {
		return ""
	}
	return string(raw)
}

// builderPayload is what a builder's webhook body reads into: its fields and
// the id the builder gives this submission.
type builderPayload struct {
	Fields []field
	ID     string
}

func readBuilder(sender string, body []byte) (builderPayload, error) {
	switch sender {
	case SenderWebflow:
		return webflowPayload(body)
	case SenderTypeform:
		return typeformPayload(body)
	case SenderTally:
		return tallyPayload(body)
	}
	return builderPayload{}, fmt.Errorf("webforms: %q is not a builder", sender)
}
