// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

// equivalentContent names, per namespace and version, a pair of digests that
// build the SAME schema: the content a database may already hold, and the exact
// source content it is equivalent to. A database holding the applied half is
// admitted while the source still hashes to the current half.
//
// BOTH halves are named on purpose. Keyed on the applied digest alone, an entry
// would admit whatever the source said next — the first edit would be excused,
// and so would every later one nobody checked. Naming the current half makes an
// entry expire the moment the file changes again: the next edit is a new claim,
// and it stops migrations until somebody checks it and records the new pair.
//
// This is an enumerated list of specific past content, not a rule that forgives
// editing. Every entry asserts that two exact byte sequences build the same
// schema, and belongs here only once that has been checked by comparing the
// schemas they produce. Nothing is added here to make an edit convenient.
//
// An entry admits the version on the way DOWN as well as up, so the two byte
// sequences must also ROLL BACK compatibly — the down half that ships will run
// against a database whose up half was the other sequence. Entries whose down
// half differs need that checked separately; equal up-schemas do not imply it.
var equivalentContent = map[string]map[string][]equivalence{
	"core": {
		// The up SQL is unchanged. The down restores the validated provider vocabulary;
		// its rollback test admits the old digest and preserves live credential references.
		"1794379079": {{applied: "b072a9301b6a7adb4b5cfdc6e97461da09d02a515fbbe3725e681ee393f0e499", source: "07383c2e7cf9d9e126f51d9e9e07ae406507b5c7517b53b5a30967dd8714947c"}},
		"1794412054": {{applied: "7095201fa85775e88f7fa30e3c6e1c33a274f457f9e37118b74a51da14bfddcd", source: "a6d8d5969440802eb035846f76246f9b7e59b5f6b76b6be0a9fa11a73092cbb0"}},
		// 1789122755 is the migration that renamed the company record type's
		// columns. It rewrote stored row values BEFORE the PL/pgSQL function
		// bodies naming the renamed columns, so on a database holding rows the
		// first UPDATE fired a trigger whose body still spelled the pre-rename
		// column and the migration aborted. On an empty schema no trigger fires,
		// so that content applied cleanly and recorded the applied digest below.
		// Moving the function bodies ahead of the updates changed their order and
		// nothing else: the two files build schemas that differ in no catalog
		// object, compared as pg_dump -s of two databases migrated from them.
		"1789122755": {{
			applied: "1c6205d459b647ad8c3ae1cbac41ec19ad12d48e8186b2be2abdf8d68a59ed79",
			source:  "c3f55e24d2d4bb724f53d125e27b4d0a2410033015fd08dfd9b0a92e52898785",
		}},
		// Both versions rename the same rows. The current up and down temporarily
		// disable the last-activity triggers during that update, then re-enable
		// them. Schema dumps match after each up, each down, and the current down
		// applied to the old up; no schema change remains for an existing database.
		"1789170001": {{
			applied: "416edd1aa2a69937fc7be3c3faf77eb64367f22707d1df038a0df7f9205fe1a2",
			source:  "8d574fbe51796a818cb7ff938c766ceb152142ea4265090e2eaf445cb6c5d776",
		}},
		// The review's touch trigger wrote a version column the table lacks, so
		// on a database holding reviews the sweep's UPDATE aborted. The current
		// file holds that trigger off around the one UPDATE and re-enables it;
		// the down half is unchanged and no catalog object differs.
		"1790568769": {{
			applied: "3322c618c5e63b3adc0cbafd9d00a7ddb8d921fd046941747dea2e4c42a2b4a4",
			source:  "57662d25857ee38a6207b33cac92b93453f02fcf3163abf892df35a824758751",
		}},
		// The shipped body was a data backfill only: it stamped
		// capture_import.provider_received_at from stored originals and relabelled
		// acquisitions and notice cases. It created no catalog object, so the
		// no-op that replaced it builds the same schema; the down half of either
		// leaves the schema unchanged too. It was emptied because its run time grew
		// with the mailbox and outlasted the api's boot window (margince#6692);
		// the body moved unchanged to migrations/datafix/ as an operator task.
		"1790871111": {{
			applied: "95c9f28caa40048fcdf856f131e886bdc08ce249dfee2ce731d6244a3fe71f4b",
			source:  "e96ba4f65cfffb7aad40b1eb7825602e8b2fb53015b2ab9d5f6511786de51b5c",
		}},
		// The applied half added the band CHECK validated, the current one `NOT VALID`;
		// 1793810020 validates it, a no-op where it already is. The down half is unchanged.
		"1793765139": {{
			applied: "01cb044b9619fe806a7cf86fee9feeccd2c1c2687dc41362777538bb113e9649",
			source:  "b6f75f1f2ece9b3ce25de15c04bdc2855fa75b516ba54f3403c91c0fb793df3a",
		}},
		// Only the down half differs: it restores the older vocabulary `NOT VALID`, so
		// a rollback takes no scan under ACCESS EXCLUSIVE. Checked for rollback: the
		// one schema difference is that flag, the down rewrites the value it refuses
		// first, and the next up replaces the check.
		"1793784484": {{
			applied: "d5ec53c23e2347fadc65f9bdd9d63e9dfae40c6897c1e1b8c59f046c8e99eb0e",
			source:  "76336c558410fc1ee67862f22fb4689b5f9655917af35617f55c11b31f154669",
		}},
		// a_unit_sql_runs_as_the_ext_runtime_role first shipped as 1794364497 and
		// resetting the ext_runtime role always. The source resets it only when an
		// attribute is wrong. Both leave the same role and grants, and the down
		// half is unchanged. The applied half is that first text under 1794364497:
		// a database holding it reaches this version through renumbered.txt.
		"1794412052": {{
			applied: "8e277071e58fc236abf58acb178be1c8efbeb0014ef3440f49db0809546e401c",
			source:  "1822dfeae5b85103a5fb20e819c4f4258c0d3aa4c5b4ef5ea71130c057a53024",
		}},
		// The applied pair rewrote decisions holding the retired basis to consent
		// and refused a stored basis row, then validated both checks. The current
		// pair keeps every row and validates a check only when no row breaks it.
		// A database that applied the old pair holds no such row, so both checks
		// end validated with the same lists. The down halves differ in comments.
		"1795388887": {{
			applied: "0ab57237649bd663379393576642774b89df7bc0312e6842025d6b905b38351f",
			source:  "78902b05da28b4f0c85fef69c26058f08ba6867b810e0795eb2ab1b08a0d92b0",
		}},
		"1795388888": {{
			applied: "d641a312d5aaefa1f060757a73f000fcd7045c7ac855fc6d18f2668a24c928e7",
			source:  "15cea25a279872b10bc758ef84c7c9379365648ee9ec844d573d84285a14ecd8",
		}},
	},
}

// ContentAdmitted answers whether a migrate would admit a database that
// recorded applied as this migration's content: the digests agree, or the pair
// is one of the checked equivalences above.
//
// Exported because the test-database head probe asks the same question, and a
// probe stricter than the migrator is not a safer probe — it reports a template
// the migrator would reuse as not-at-head and rebuilds it. Both doors read the
// list through here rather than each looping over it.
//
// Held by: TestEquivalentContentIsAdmittedAndNothingElseIs
// (backend/internal/platform/dbmigrate/dbmigrate_test.go), which puts the same
// three cases to this and to assertContentMatches and compares their verdicts.
func ContentAdmitted(namespace string, m Migration, applied string) bool {
	return applied == Digest(m) || equivalentTo(namespace, m, applied)
}

// equivalentTo answers whether a checked entry for m's version names applied
// as building the schema m's source builds today.
func equivalentTo(namespace string, m Migration, applied string) bool {
	return pairedWith(namespace, m.Version, Digest(m), applied)
}

// pairedWith answers whether a checked entry for version names applied as
// building what current builds. A replaced version whose file is gone has no
// source to hash, so its manifest digest stands in as current.
func pairedWith(namespace, version, current, applied string) bool {
	for _, eq := range equivalentContent[namespace][version] {
		if applied == eq.applied && current == eq.source {
			return true
		}
	}
	return false
}

// equivalence is one checked claim that two byte sequences build one schema.
type equivalence struct {
	applied string // what the database recorded
	source  string // what the source must still hash to for that to hold
}
