// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An optional contract field is a pointer, and the three below make one from a
// literal — the types the suites actually pass optionally.
//
// Here rather than in whichever suite first needed one, which is where all
// three were: strPtr in authz_integration_test.go, boolPtr in
// installation_integration_test.go, int64Ptr in companyrollup. A generic helper
// in a subject file is findable only by someone already reading that subject,
// so the suites that could not find it wrote their own — collections had a
// second strPtr, capture a second boolPtr, both identical to the originals and
// to each other. That is the shape this package exists to stop.
//
// Exported because the suite packages are the callers that could not reach the
// originals; the parent's own files read them unqualified either way.

// StrPtr carries a string into an optional field.
func StrPtr(s string) *string { return &s }

// BoolPtr carries a bool into an optional field, including a deliberate false —
// which is the case a bare omission cannot express.
func BoolPtr(v bool) *bool { return &v }

// Int64Ptr carries an int64 into an optional field.
func Int64Ptr(v int64) *int64 { return &v }
