// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The copy for the import verbs. It carries one thing the schema cannot: which
// of `lead` and `person` a given file wants, since both are valid and the
// difference is about where the file came from rather than what it contains.

var previewImportCopy = toolCopy{
	Purpose: "Bring a spreadsheet in: send the CSV as text with a `mapping` saying what each " +
		"column is, and this checks every row against the workspace and reports what importing " +
		"it would do.",
	Limits: "Writes nothing. A column header is matched to a field NAME and never guessed, so an " +
		"ordinary header row — `name`, `company`, `city` — places nothing without a mapping and is " +
		"refused with the field list to map onto. `object` is company, person or lead. Use `person` for a file " +
		"the business already knows — a migration off another CRM, a corrected export coming back. " +
		"Use `lead` for a machine-sourced list nobody has worked yet; those land unworked and a " +
		"human promotes them. A row naming a record already here is counted in `duplicates`, and " +
		"created unless on_duplicate is skip — except a person whose email is already held, which " +
		"is always refused, because an email is a real key. A company's Website or Domain column maps to " +
		"`domain`, which is what identifies a company — import it and dedupe stops guessing from names. " +
		"To link people to their employers, map the company column to " +
		"`company_name` — import the companies FIRST, because a name that matches nothing links " +
		"nothing and says so. To CORRECT companies rather than add " +
		"them, map a column to `id`, then give a row the id of the company it corrects — read them " +
		"out first. A row whose `id` is EMPTY is a new company, so one file may both correct and add.",
	Instead: "create_record for one record you already know.",
	Retain: "Keep the run_id. The counts it answers — created, duplicates, skipped — and the " +
		"mapping it settled on are what the person weighs, so report both: a column this placed by " +
		"a name they did not write is a decision they did not make.",
}

var readImportRunCopy = toolCopy{
	Purpose: "Where one import got to: awaiting approval, running, done, or stopped.",
	Limits:  "A stopped run names the row it stopped at and can resume there.",
}

var readImportReportCopy = toolCopy{
	Purpose: "What an import will do, or did: rows created, updated, failed, unusable, duplicates.",
	Limits:  "These counts are what a person weighs before committing. Same shape before and after.",
}

// The approval `awaiting_approval` names is the PERSON's, and this tool is not
// staged: the same passport holds the dry run and the commit, so nothing stops
// the caller answering its own question. Two of three measured runs previewed,
// read the report and committed in one turn, and the run that stopped is the
// one the person would have thanked.
//
// What makes waiting right is on the surface already and was never joined to
// it: this write cannot be undone from here. So the copy says whose answer it
// is, and says the one case that does not need a fresh one — a person who has
// already been through the file and said to load it has approved it, and asking
// again is not diligence.
var commitImportCopy = toolCopy{
	Purpose: "Write a checked import into the workspace. The dry run is the check; this commits " +
		"when it answers.",
	Limits: "Only from awaiting_approval, which is the PERSON's approval and not this call's to " +
		"give: nothing stages it, and an import cannot be undone from here — undoing one needs the " +
		"web app. Put the dry run's counts in front of them and let them say go. The exception is " +
		"a person who has already been through the file and asked for it to be loaded; they have " +
		"approved it, and asking a second time is not diligence.",
	Instead: "read_import_report first, and report what it says — numbers nobody read are not a check.",
}
