// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"strings"
	"testing"
)

const (
	repoCatalog  = "../../api/worklist-kinds.yaml"
	repoLocales  = "../../../frontend/src/i18n"
	repoFrontend = "../../../frontend/src/lib/worklistkinds.gen.ts"
)

func TestTheFrontendKindTableIsCurrent(t *testing.T) {
	catalog, err := loadCatalog(repoCatalog)
	if err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(repoFrontend)
	if err != nil {
		t.Fatal(err)
	}
	if string(render(catalog)) != string(onDisk) {
		t.Errorf("%s is stale: run make gen-worklist", repoFrontend)
	}
}

func TestEveryKindIsTitledInEveryLocale(t *testing.T) {
	catalog, err := loadCatalog(repoCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkTitles(catalog, repoLocales); err != nil {
		t.Error(err)
	}
}

func TestAKindWithNoTitleIsRefused(t *testing.T) {
	catalog, err := loadCatalog(repoCatalog)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, file := range locales {
		if err := os.WriteFile(dir+"/"+file, []byte(`export const en = { "worklist.untitled.task": "Task" };`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err = checkTitles(catalog, dir)
	if err == nil || !strings.Contains(err.Error(), "worklist.untitled.customer_waiting") {
		t.Errorf("got %v, want a refusal naming the missing title", err)
	}
}

func TestAnExtensionKindIsTitledUnderItsUnit(t *testing.T) {
	catalog, err := loadCatalog(repoCatalog)
	if err != nil {
		t.Fatal(err)
	}
	kind := catalog["task"]
	kind.Name, kind.ExtensionKey = "ext_vn.einvoice_rejected", true
	if got := titleKey(kind); got != "extVn.worklist.einvoice_rejected" {
		t.Errorf("title key %q", got)
	}
}
