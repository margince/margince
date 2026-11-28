// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestExplicitSalutationsRoundTripThroughTheAPIAndCSV(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	for _, object := range []string{"contact", "lead"} {
		body := AnyMap{"full_name": "Explicit choice " + object, "source": "manual", "salutation": "ms"}
		if object == "contact" {
			body["emails"] = []AnyMap{{"email": "contact@example.test", "is_primary": true}}
		} else {
			body["email"] = "lead@example.test"
		}
		var out struct {
			ID         string  `json:"id"`
			Salutation *string `json:"salutation"`
		}
		if status := e.Call(t, "POST", "/v1/"+object+"s", body, nil, &out); status != http.StatusCreated || out.Salutation == nil || *out.Salutation != "ms" {
			t.Fatalf("%s: status=%d reply=%+v", object, status, out)
		}
		file, _ := exportAll(t, e, object)
		rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(file, []byte{0xef, 0xbb, 0xbf}))).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		column := slices.Index(rows[0], "salutation")
		if column < 0 {
			t.Fatalf("%s CSV lacks salutation: %v", object, rows[0])
		}
		found := false
		for _, row := range rows[1:] {
			if row[column] == "ms" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s CSV lost the stored choice", object)
		}
		body["salutation"] = "inferred_from_name"
		body["full_name"] = "Invalid choice " + object
		if object == "contact" {
			body["emails"] = []AnyMap{{"email": "bad-contact@example.test", "is_primary": true}}
		} else {
			body["email"] = "bad-lead@example.test"
		}
		if status := e.Call(t, "POST", "/v1/"+object+"s", body, nil, nil); status != http.StatusUnprocessableEntity {
			t.Fatalf("%s invalid choice: %d", object, status)
		}
	}
}
