// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package extdelivery holds what a unit's bus delivery needs beyond the core
// port: the member it runs as, the roster door and the custom values.
// It takes the compose seams as arguments.
// It imports none of that package.
package extdelivery

// The custom values a unit offers for a company or a deal. Landing a record and
// a handler's SetCustomFields both decide what to write here, so the two cannot
// disagree about which value a user owns.

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
)

// ValuesToWrite answers the offered custom fields that differ from the record
// and that no user last edited, and names the ones a user owns. A field has no
// pairing like amount and currency, so each is held back on its own. held names
// the fields of a patch a user last edited.
func ValuesToWrite(
	ctx context.Context, current, offered map[string]any, held func(context.Context, map[string]any) ([]string, error),
) (map[string]any, []string, error) {
	if len(offered) == 0 {
		return nil, nil, nil
	}
	// The stored value reads back as decoded JSON. The offer is put through JSON
	// too, or a []string would never equal the []any it was saved as.
	raw, err := json.Marshal(offered)
	if err != nil {
		return nil, nil, fmt.Errorf("compose: encoding the offered custom fields: %w", err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, nil, fmt.Errorf("compose: reading the offered custom fields back: %w", err)
	}
	changed := map[string]any{}
	for key, value := range normalized {
		if !reflect.DeepEqual(current[key], value) {
			changed[key] = value
		}
	}
	owned, err := held(ctx, changed)
	if err != nil {
		return nil, nil, err
	}
	for _, key := range owned {
		delete(changed, key)
	}
	slices.Sort(owned)
	return changed, owned, nil
}
