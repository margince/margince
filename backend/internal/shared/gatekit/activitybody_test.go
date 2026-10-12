// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import "testing"

func TestEverySpellingOfEmptyingAnActivityBodyIsRecognised(t *testing.T) {
	t.Parallel()
	for _, statement := range []string{
		`UPDATE activity SET body = NULL WHERE id = $1`,
		`update "activity" set body=null where id = $1`,
		`UPDATE public.activity a SET a.body = NULL`,
		`UPDATE ONLY activity SET subject = NULL, body = NULL`,
	} {
		if !DestroysActivityBody(statement) {
			t.Errorf("not recognised as emptying an activity body: %s", statement)
		}
	}
	for _, statement := range []string{
		`UPDATE activity SET audience = $2 WHERE id = $1`,
		`UPDATE comms_outbound SET body = NULL`,
		`UPDATE activity_participant SET body = NULL`,
		`SELECT id FROM activity WHERE body = NULL`,
	} {
		if DestroysActivityBody(statement) {
			t.Errorf("recognised as emptying an activity body, and it does not: %s", statement)
		}
	}
}
