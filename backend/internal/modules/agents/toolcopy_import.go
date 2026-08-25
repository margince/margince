// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The copy for the import verbs. It carries one thing the schema cannot: which
// of `lead` and `person` a given file wants, since both are valid and the
// difference is about where the file came from rather than what it contains.

var previewImportCopy = toolCopy{
	Purpose: "Bring a spreadsheet in: send the CSV as text and this checks every row " +
		"against the workspace and reports what importing it would do.",
	Limits: "Writes nothing. `object` is organization, person or lead. Use `person` for a file " +
		"the business already knows — a migration off another CRM, a corrected export coming back. " +
		"Use `lead` for a machine-sourced list nobody has worked yet; those land unworked and a " +
		"human promotes them. A row naming a record already here is counted in `duplicates`, and " +
		"created unless on_duplicate is skip — except a person whose email is already held, which " +
		"is always refused, because an email is a real key. To CORRECT companies rather than add " +
		"them, map a column to `id`, then give a row the id of the company it corrects — read them " +
		"out first. A row whose `id` is EMPTY is a new company, so one file may both correct and add.",
	Instead: "create_record for one record you already know.",
	Retain:  "run_id, and `duplicates` — give the user both numbers before committing.",
}

var readImportRunCopy = toolCopy{
	Purpose: "Where one import got to: awaiting approval, running, done, or stopped.",
	Limits:  "A stopped run names the row it stopped at and can resume there.",
}

var readImportReportCopy = toolCopy{
	Purpose: "What an import will do, or did: rows created, updated, failed, unusable, duplicates.",
	Limits:  "These counts are what a person approves. Same shape before and after.",
}

var commitImportCopy = toolCopy{
	Purpose: "Write a checked import into the workspace, once a person approves.",
	Limits:  "Only from awaiting_approval. Undoing one needs the web app.",
	Instead: "read_import_report first; nobody should approve what they have not read.",
}
