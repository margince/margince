// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package orgscan is the account scan: the model's reading of one account
// for one reader, saying what needs a person and quoting the words it read.
//
//	Tables owned: org_scan (the per-user scan row: the read in flight and
//	the last findings that settled).
//
// Three rules shape it, the same three the account brief holds.
//
// PER READER, because visibility is per reader. The input is the reader's
// own composite read plus the message words their audience admits, so a
// colleague's scan would disclose records this reader cannot open. One row
// per (reader, account); the dismissals are theirs too.
//
// ON DEMAND, NEVER A SWEEP. Opening the account page asks for the scan; the
// stored one is served while its fingerprint still matches the account, and
// the account is read again at most once an hour per reader. A busy inbox
// must not re-read the account on every message, and a reader who never
// opens an account never pays for it.
//
// THE RULES ARE THE FLOOR. The 360's own advice runs on every read and is
// merged with the model's stored findings under one fingerprint vocabulary,
// so a dismissal holds across both and a deployment with no lane still gets
// the rows the records support. A finding the model raises is dropped whole
// when it cites a record the reader was not given or quotes words the cited
// message does not contain — never shown with its citation stripped.
package orgscan
