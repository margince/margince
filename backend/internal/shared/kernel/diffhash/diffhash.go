// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package diffhash spells the ONE canonicalization a diff_hash carries
// (ADR-0036): decode into interface maps, re-marshal — which sorts keys
// at every depth — and hash those bytes. Staging, redemption, and
// modify-then-approve all hash through here, so "identical call" is a
// property of content, never of whitespace or a client's key order.
package diffhash

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Canonical re-serializes one JSON object canonically and returns the
// bytes together with their diff_hash.
func Canonical(raw json.RawMessage) (json.RawMessage, string, error) {
	m, err := DecodeObject(raw)
	if err != nil {
		return nil, "", err
	}
	return Object(m)
}

// DecodeObject decodes exactly one JSON object into an untyped map WITHOUT
// passing its numbers through float64.
//
// A plain json.Unmarshal into `any` decodes every number as a float64, which
// represents an integer exactly only below 2^53. Above that the canonicalization
// is lossy and silent: 9007199254740993 re-serializes as 9007199254740992, so two
// different proposed changes canonicalize to the same bytes and take the same
// diff_hash — and a diff_hash is the idempotency key staging reads to decide that
// a call is already pending. The second change is then dropped as a duplicate of
// the first. The rounded value is also the one that TRAVELS: the canonical bytes
// are what modify-then-approve re-decodes and a resumed agent run acts on.
//
// UseNumber keeps each number as its exact decimal text, and re-marshalling a
// json.Number emits those digits back, at every depth.
//
// Object-only, and no trailing data: a proposed change is ONE object rather than a
// stream, and reading the first of several values would hash a payload the rest of
// the input contradicts.
func DecodeObject(raw json.RawMessage) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var m map[string]any
	if err := decoder.Decode(&m); err != nil {
		return nil, fmt.Errorf("diffhash: payload is not a JSON object: %w", err)
	}
	if m == nil {
		return nil, errors.New("diffhash: payload is not a JSON object: null")
	}
	if err := noTrailingData(decoder); err != nil {
		return nil, err
	}
	return m, nil
}

// DecodeValue is DecodeObject for a payload that need not be an object — a member
// of a change being compared against another, which may be any JSON value.
//
// Same reason, same guarantee: a comparison that decodes through float64 reports
// two amounts differing only above 2^53 as equal, so an edit that changed one reads
// as having changed nothing.
//
//craft:ignore naked-any the return IS an arbitrary decoded JSON value — a member of a proposed change, which the contract declares open by kind, so a concrete type would be a claim about payload shape this must not make
func DecodeValue(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var v any
	if err := decoder.Decode(&v); err != nil {
		return nil, fmt.Errorf("diffhash: payload is not JSON: %w", err)
	}
	if err := noTrailingData(decoder); err != nil {
		return nil, err
	}
	return v, nil
}

// noTrailingData refuses a payload carrying anything after its first value.
//
// A second Decode rather than More(): More reports `err == nil && c != ']' && c !=
// '}'`, so a stray closing delimiter — `{"a":1}}` or `{"a":1}]` — makes it answer
// false and the trailing byte is dropped silently. Reading again and requiring EOF
// costs one more Decode of an empty remainder and refuses what More cannot see.
func noTrailingData(decoder *json.Decoder) error {
	var rest json.RawMessage
	if err := decoder.Decode(&rest); !errors.Is(err, io.EOF) {
		return errors.New("diffhash: payload carries more than one JSON value")
	}
	return nil
}

// Object canonicalizes an already-decoded object.
func Object(m map[string]any) (json.RawMessage, string, error) {
	canonical, err := json.Marshal(m)
	if err != nil {
		return nil, "", fmt.Errorf("diffhash: canonicalize: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(sum[:]), nil
}
