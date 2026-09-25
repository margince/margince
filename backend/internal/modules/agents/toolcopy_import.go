// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The copy for the import verbs. It carries one thing the schema cannot: which
// of `lead` and `contact` a given file wants, since both are valid and the
// difference is about where the file came from rather than what it contains.

var previewImportCopy = toolCopy{
	Purpose: "Bring a spreadsheet in: send the CSV as text with a `mapping` saying what each " +
		"column is, and this checks every row against the workspace and reports what importing " +
		"it would do.",
	Limits: "Writes nothing. `object` is company, contact or lead. Use `contact` for a file " +
		"the business already knows — a migration off another CRM, a corrected export coming back. " +
		"Use `lead` for a machine-sourced list nobody has worked yet; those land unworked and a " +
		"human promotes them. A row naming a record already here is counted in `duplicates`, and " +
		"created unless on_duplicate is skip — except a contact whose email is already held, which " +
		"is always refused, because an email is a real key. A company's Website or Domain column maps to " +
		"`domain`, which is what identifies a company — import it and dedupe stops guessing from names. " +
		"To link contacts to their employers, map the company column to " +
		"`company_name` — import the companies FIRST, because a name that matches nothing links " +
		"nothing and says so. To CORRECT companies rather than add " +
		"them, map a column to `id`, then give a row the id of the company it corrects — read them " +
		"out first. A row whose `id` is EMPTY is a new company, so one file may both correct and add.",
	Instead: "create_record for one record you already know.",
	Retain: "Keep the run_id. The counts it answers — created, duplicates, skipped — and the " +
		"mapping it settled on are what the contact weighs, so report both: a column this placed by " +
		"a name they did not write is a decision they did not make.",
}

var readImportRunCopy = toolCopy{
	Purpose: "Where one import got to: awaiting approval, running, done, or stopped.",
	Limits:  "A stopped run names the row it stopped at and can resume there.",
}

var readImportReportCopy = toolCopy{
	Purpose: "What an import will do, or did: rows created, updated, failed, unusable, duplicates.",
	Limits:  "These counts are what a contact weighs before committing. Same shape before and after.",
}

// A commit writes the file and cannot be undone from this surface. That, and
// not the run's state name, is what earns the dry run its turn.
//
// The copy does not call awaiting_approval an approval, because it is not one:
// the same passport that produced the report commits it, so a sentence naming
// somebody else's answer describes a control nothing enforces — and an injected
// instruction does not respect prose. What the state does carry is sequencing,
// and refuseUncommittableRun enforces that: a run reaches it only by producing a
// report, so requiring the state is requiring the report.
var commitImportCopy = toolCopy{
	Purpose: "Write a checked import into the workspace. The dry run is the check; this commits " +
		"when it answers.",
	Limits: "Only from awaiting_approval, the state a run reaches by producing a dry-run report, " +
		"so there is always a report first. This cannot be undone from here — undoing an import " +
		"needs the web app — so show the counts to whoever asked for the file unless they have " +
		"already been through it and asked for it to be loaded.",
	Instead: "read_import_report first: numbers nobody read are not a check.",
}
