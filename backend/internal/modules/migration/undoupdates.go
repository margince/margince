// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// What a reversal cannot take back.
//
// Undo archives what a run landed. A correction to a record the run did not
// create has nothing to archive and no previous value to put back.
//
// import_record_map holds one row per identity, written by the run that first
// landed it. A later run correcting the same identity writes none, so the
// reversal walks that map, finds nothing, and reverses nothing.
//
// Counting it separates a run that did nothing from a run that left something
// standing. Somebody undoing a wrong correction was told it had been taken
// back while it stood.

import "encoding/json"

// updatesInRun counts the ROWS a run corrected rather than created, across
// every object class it touched.
//
// Rows, not records. ObjectReport.Updated is incremented once per row whose
// outcome was neither a skip nor a creation, and those counts sum to the rows
// read. Two rows naming one record therefore count twice, which is what the
// wire says.
//
// A run with no stored report counts none. A report written before this field
// existed says nothing about updates. Guessing a number from the map would
// report the opposite of the truth.
func updatesInRun(reportRaw []byte) (int, error) {
	if len(reportRaw) == 0 {
		return 0, nil
	}
	var rep Report
	if err := json.Unmarshal(reportRaw, &rep); err != nil {
		return 0, err
	}
	var updated int
	for _, obj := range rep.Objects {
		updated += obj.Updated
	}
	return updated, nil
}
