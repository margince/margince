// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package rowvalue turns the values pgx decodes from a row into values a JSON
// document can hold as they are stored. Every surface that hands rows to a
// reader, such as an export, a subject access package or a report, uses it.
package rowvalue

import (
	"bytes"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Document normalizes one driver value for a JSON document. It walks into
// jsonb objects and arrays, so a uuid inside an array is a string there too.
//
//craft:ignore naked-any a driver row value spans every SQL type a table carries; the switch narrows each
func Document(v any) any {
	switch t := v.(type) {
	case [16]byte:
		return ids.UUID(t).String()
	case []byte:
		return ByteaText(t)
	case pgtype.Interval:
		return valuerJSON{t}
	case pgtype.Bits:
		return valuerJSON{t}
	case []any:
		out := make([]any, len(t))
		for i, element := range t {
			out[i] = Document(element)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for key, element := range t {
			out[key] = Document(element)
		}
		return out
	default:
		return v
	}
}

// ByteaText is Postgres's hex text for a bytea, which an import reads back.
// Raw bytes are not text, and as JSON they would not parse at all.
func ByteaText(b []byte) string {
	return `\x` + hex.EncodeToString(b)
}

// valuerJSON spells a pgtype value as its Postgres text. Interval and Bits have
// no JSON form of their own and would marshal as their struct fields.
type valuerJSON struct{ driver.Valuer }

func (v valuerJSON) MarshalJSON() ([]byte, error) {
	text, err := v.Value()
	if err != nil {
		return nil, err
	}
	return json.Marshal(text)
}

// CompactJSON is v as one line of JSON. It leaves & < > as stored, because the
// reader is a file, not an HTML page.
//
//craft:ignore naked-any v is a Document value, whose shape the stored jsonb decides
func CompactJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(Document(v)); err != nil {
		return "", err
	}
	return string(bytes.TrimSuffix(buf.Bytes(), []byte("\n"))), nil
}

// Reader reads rows like pgx.Rows.Values, except that json and jsonb numbers
// arrive as json.Number. pgx decodes them as float64, which changes every
// integer above 2^53. A Reader is for one query; it is not safe to share.
type Reader struct {
	types *pgtype.Map
}

// NewReader builds a Reader whose json codecs keep each number's stored digits.
func NewReader() *Reader {
	types := pgtype.NewMap()
	jsonType := &pgtype.Type{
		Name: "json", OID: pgtype.JSONOID,
		Codec: &pgtype.JSONCodec{Marshal: json.Marshal, Unmarshal: unmarshalExact},
	}
	jsonbType := &pgtype.Type{
		Name: "jsonb", OID: pgtype.JSONBOID,
		Codec: &pgtype.JSONBCodec{Marshal: json.Marshal, Unmarshal: unmarshalExact},
	}
	types.RegisterType(jsonType)
	types.RegisterType(jsonbType)
	types.RegisterType(&pgtype.Type{Name: "_json", OID: pgtype.JSONArrayOID, Codec: &pgtype.ArrayCodec{ElementType: jsonType}})
	types.RegisterType(&pgtype.Type{Name: "_jsonb", OID: pgtype.JSONBArrayOID, Codec: &pgtype.ArrayCodec{ElementType: jsonbType}})
	return &Reader{types: types}
}

//craft:ignore naked-any the codec's Unmarshal signature is encoding/json's
func unmarshalExact(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(v)
}

// Values is the current row's values, json columns decoded by this Reader.
//
//craft:ignore naked-any a row's values are []any by pgx's own contract
func (r *Reader) Values(rows pgx.Rows) ([]any, error) {
	values, err := rows.Values()
	if err != nil {
		return nil, err
	}
	raw := rows.RawValues()
	for i, field := range rows.FieldDescriptions() {
		switch field.DataTypeOID {
		case pgtype.JSONOID, pgtype.JSONBOID, pgtype.JSONArrayOID, pgtype.JSONBArrayOID:
		default:
			continue
		}
		decoded, ok := r.types.TypeForOID(field.DataTypeOID)
		if !ok {
			return nil, fmt.Errorf("rowvalue: no codec for json type %d", field.DataTypeOID)
		}
		values[i], err = decoded.Codec.DecodeValue(r.types, field.DataTypeOID, field.Format, raw[i])
		if err != nil {
			return nil, fmt.Errorf("rowvalue: column %s: %w", field.Name, err)
		}
	}
	return values, nil
}
