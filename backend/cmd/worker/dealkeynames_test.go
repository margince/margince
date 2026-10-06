// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
)

func TestTheExportIsReadRowByRow(t *testing.T) {
	got, err := readKeyNamedDeals(strings.NewReader(
		"source_system,source_key,source_title\nlegacy,acme-q3,Acme Q3 renewal\nlegacy,beta-1,\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []deals.KeyNamedDeal{
		{SourceSystem: "legacy", SourceKey: "acme-q3", SourceTitle: "Acme Q3 renewal"},
		{SourceSystem: "legacy", SourceKey: "beta-1"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("read %+v, want %+v", got, want)
	}
}

func TestAnExportTheRepairCannotTrustIsRefusedWhole(t *testing.T) {
	for name, body := range map[string]string{
		"wrong header":   "system,key,title\nlegacy,acme-q3,\n",
		"empty key":      "source_system,source_key,source_title\nlegacy,,Acme\n",
		"empty system":   "source_system,source_key,source_title\n,acme-q3,Acme\n",
		"duplicate key":  "source_system,source_key,source_title\nlegacy,acme-q3,A\nlegacy,acme-q3,B\n",
		"no rows at all": "source_system,source_key,source_title\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readKeyNamedDeals(strings.NewReader(body)); err == nil {
				t.Error("accepted an export it should refuse")
			}
		})
	}
}

func TestARefusedRowIsNamedByItsLine(t *testing.T) {
	_, err := readKeyNamedDeals(strings.NewReader(
		"source_system,source_key,source_title\nlegacy,acme-q3,A\nlegacy,,B\n"))
	if err == nil || !strings.Contains(err.Error(), "line 3: source_key is empty") {
		t.Errorf("refusal %v does not name line 3", err)
	}
}

func TestASpreadsheetsByteOrderMarkDoesNotSpoilTheHeader(t *testing.T) {
	got, err := readKeyNamedDeals(strings.NewReader(
		"\uFEFFsource_system,source_key,source_title\nlegacy,acme-q3,Acme\n"))
	if err != nil || len(got) != 1 {
		t.Errorf("read %+v, %v; want the one row", got, err)
	}
}

func TestADryRunReportSaysNothingWasWritten(t *testing.T) {
	var out bytes.Buffer
	err := writeKeyNameReport(&out, []deals.KeyNameResult{
		{Entry: deals.KeyNamedDeal{SourceKey: "acme-q3"}, To: "Acme · Proposal", Outcome: deals.KeyNameWouldRename},
		{Entry: deals.KeyNamedDeal{SourceKey: "beta-1"}, Outcome: deals.KeyNameNoCompany},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"would-rename", "acme-q3", "Acme · Proposal", "no-company", "Nothing was written", "--apply"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}
