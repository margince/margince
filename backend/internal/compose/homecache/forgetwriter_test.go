// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package homecache

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestASuccessOrAConflictEndsTheWritersCachedHome(t *testing.T) {
	for status, want := range map[int]bool{
		http.StatusOK: true, http.StatusNoContent: true, http.StatusConflict: true,
		http.StatusForbidden: false, http.StatusUnprocessableEntity: false, http.StatusInternalServerError: false,
	} {
		forgot := 0
		w := NewForgettingWriter(httptest.NewRecorder(), func(int) { forgot++ })
		w.WriteHeader(status)
		w.WriteHeader(status)
		if wantCount := map[bool]int{true: 1, false: 0}[want]; forgot != wantCount {
			t.Errorf("a %d answer ended the cache %d times, want %d", status, forgot, wantCount)
		}
	}
}
