// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package storedobjectreap is the part of the stored-object reap that needs no
// database. It sorts queued keys into orphans and adopted keys, and walks the
// ledger batch after batch. compose wires the owners and runs the job.
package storedobjectreap

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/storedobjects"
)

// Owner answers which of the keys its table does not carry.
type Owner func(ctx context.Context, keys []string) ([]string, error)

// Verdict is what one pass over the ledger established.
type Verdict struct {
	// Orphans are keys whose owning row never arrived, or is gone: bytes to delete.
	Orphans []string
	// Adopted are keys an owning row still carries. Their bytes stay and their
	// intent is retired. Left in the ledger, each would take a slot in every later
	// batch and starve the orphans behind it.
	Adopted []string
}

// Adjudicate groups what the ledger holds and asks each kind's owner.
//
// A kind with no owner is reported, and the pass goes on for every other kind.
// Stopping would leave every owned kind uncollected because of one unowned kind.
// The fault is still returned, so it reaches an operator.
func Adjudicate(
	ctx context.Context, owners map[storedobjects.Kind]Owner, provisional []storedobjects.Provisional,
) (Verdict, error) {
	byKind := map[storedobjects.Kind][]string{}
	for _, p := range provisional {
		byKind[p.Kind] = append(byKind[p.Kind], p.Key)
	}
	var out Verdict
	var unowned []storedobjects.Kind
	for kind, keys := range byKind {
		owner, ok := owners[kind]
		if !ok {
			unowned = append(unowned, kind)
			continue
		}
		unreferenced, err := owner(ctx, keys)
		if err != nil {
			return out, err
		}
		out.Orphans = append(out.Orphans, unreferenced...)
		out.Adopted = append(out.Adopted, keysExcept(keys, unreferenced)...)
	}
	if len(unowned) > 0 {
		return out, fmt.Errorf("the stored-object sweep has no owner for kind(s) %v, so their keys cannot be adjudicated", unowned)
	}
	return out, nil
}

// keysExcept is the complement: everything the owner did not call unreferenced.
func keysExcept(keys, unreferenced []string) []string {
	gone := make(map[string]bool, len(unreferenced))
	for _, key := range unreferenced {
		gone[key] = true
	}
	var kept []string
	for _, key := range keys {
		if !gone[key] {
			kept = append(kept, key)
		}
	}
	return kept
}

// UnreferencedByAll is a key every owner calls unreferenced. A logo is owned
// by the company or entity wearing it and by every sent offer that printed it.
// Its bytes stay while any of them names it.
func UnreferencedByAll(owners ...Owner) Owner {
	return func(ctx context.Context, keys []string) ([]string, error) {
		left := keys
		for _, owner := range owners {
			var err error
			if left, err = owner(ctx, left); err != nil {
				return nil, err
			}
			if len(left) == 0 {
				return nil, nil
			}
		}
		return left, nil
	}
}
