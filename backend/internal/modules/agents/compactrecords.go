// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const compactRecordsProperty = `"compact":{"type":"boolean","description":"Activity identifying facts and body_length without long prose. Default false returns full records."},`

func compactActivityRecord(record *wireRecord) error {
	if record.RecordType != string(datasource.EntityActivity) {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(record.Fields, &fields); err != nil {
		return fmt.Errorf("read activity fields for compact output: %w", err)
	}
	compact := make(map[string]json.RawMessage)
	//nolint:goconst // The wire projection repeats field names from the per-language label maps.
	for _, name := range []string{kindArg, "subject", "direction", "participants", "occurred_at", "occurred_at_precision", "occurred_precision", "source_system", "source", "owner_id", "content_state", "withheld_reason"} {
		if value, present := fields[name]; present {
			compact[name] = value
		}
	}
	if body, present := fields["body"]; present {
		var text *string
		if err := json.Unmarshal(body, &text); err != nil {
			return fmt.Errorf("read activity body length: %w", err)
		}
		if text != nil {
			length, err := json.Marshal(utf8.RuneCountInString(*text))
			if err != nil {
				return err
			}
			compact["body_length"] = length
		}
	}
	encoded, err := json.Marshal(compact)
	if err != nil {
		return err
	}
	record.Fields = encoded
	return nil
}
