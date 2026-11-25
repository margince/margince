// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"encoding/json"
	"errors"
	"testing"
)

const rlo = "‮"

func TestANameThatReversesItsOrderIsRefusedWhereverItSits(t *testing.T) {
	for _, body := range []string{
		`{"full_name":"Ada ` + rlo + `gnp.exe"}`,
		`{"contact":{"first_name":"Ada","last_name":"` + rlo + `x"}}`,
		`{"items":[{"title":"ok"},{"name":"⁦Ada⁩"}]}`,
	} {
		err := nameOverrideRefusal(json.RawMessage(body))
		var detailed *DetailedError
		if !errors.As(err, &detailed) {
			t.Errorf("%s was accepted, and its name would show in another order than stored", body)
		}
	}
}

func TestOtherTextKeepsWhatWasTyped(t *testing.T) {
	for _, body := range []string{
		`{"full_name":"José Ðội"}`,
		`{"body":"` + rlo + ` in a note is the writer's own"}`,
		`{"name":"Ada","note":"` + rlo + `"}`,
		`{"tags":["` + rlo + `"],"name":"Ada"}`,
		`{"title":null,"name":3}`,
		`not json`,
	} {
		if err := nameOverrideRefusal(json.RawMessage(body)); err != nil {
			t.Errorf("%s was refused: %v", body, err)
		}
	}
}
