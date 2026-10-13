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
		"out first. A row whose `id` is EMPTY is a new company, so one file may both correct and add. " +
		"Files that link to each other go together in `files`, at most one per type, instead of " +
		"`object` and `csv`: a `ref` column is a row's own key, and a link such as a contact's " +
		"`company_ref` names a company's ref. " +
		"read_import with view format lists each file's columns.",
	Instead: "create_record for one record you already know.",
	Retain: "Keep the run_id. The counts it answers — created, duplicates, skipped — and the " +
		"mapping it settled on are what the user weighs, so report both: a column this placed by " +
		"a name they did not write is a decision they did not make.",
}

var readImportReportCopy = toolCopy{
	Purpose: "One import run: its state, its files, and once validated what it will do or did: " +
		"rows created, updated, failed, unusable, duplicates.",
	Limits: "The report is absent while the run is pending or validating. A stopped run names the " +
		"row it stopped at and can resume there. The report keeps one shape before and after the commit.",
}

var describeImportFormatCopy = toolCopy{
	Purpose: "The files an import takes: their order, each file's columns with what goes in them, " +
		"and the rules that key a row and link files.",
	Limits: "Includes this workspace's custom fields. Writes nothing.",
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
	Purpose: "Write a checked import into the workspace. The dry run is the check; this starts the " +
		"write and answers at once with the run running.",
	Limits: "Only from awaiting_approval, the state a run reaches by producing a dry-run report, " +
		"so there is always a report first. This cannot be undone from here — undoing an import " +
		"needs the web app. A request to get a file in is not a review of it: unless the asker " +
		"says they have already been through these rows, show the counts and the mapping and wait " +
		"for their go-ahead.",
	Instead: "Use read_import for the report first: numbers nobody read are not a check.",
	Retain: "The write goes on after this answers. Read read_import's report until its state is " +
		"complete or failed, then report what landed: the counts and the mapping the run used, " +
		"each column by the field it went to.",
}

var readImportCopy = toolCopy{
	Purpose: "Read one import run, or the format an import takes. report, by run_id: the run's state " +
		"and files, and once validated what it will do or did, in rows created, updated, failed, " +
		"unusable and duplicates. format: each file's columns, their order and the key rules.",
	Limits: "The report's counts are what the user weighs before committing. After a commit, read " +
		"report again until the state is complete or failed.",
}
