// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// jobsContract is a merged jobs.yaml holding the core queues block a fragment
// may ride and whatever kinds a case declares.
func jobsContract(kinds string) map[string][]byte {
	return map[string][]byte{jobsContractBase: []byte(`
queues:
  default: {max_workers: 5}
  ai_capture: {max_workers: 2, reason: long}
kinds:
  close_date_sweep:
    role: dispatcher
    go_type: CloseDateSweepArgs
    queue: default
    timeout: 2m
    opts_owner: caller
    cadence: 24h
` + kinds)}
}

func demoUnits() []extensionUnit { return []extensionUnit{{Name: "demo"}} }

const wellFormedPair = `
  ext_demo_refresh:
    job: refresh
    role: dispatcher
    queue: default
    timeout: 1m
    cadence: 6h
    tier: auto_execute
    scope: read
  ext_demo_refresh_ws:
    job: refresh
    role: worker
    queue: default
    timeout: 5m
    max_attempts: 3
`

// TestExtensionJobsReadsAPairOutOfTheMergedContract is the happy path: the two
// kinds a scheduled job compiles to are read back out of the document a client
// is handed, not out of the fragment a merge could still have refused.
func TestExtensionJobsReadsAPairOutOfTheMergedContract(t *testing.T) {
	decls, err := extensionJobs(demoUnits(), jobsContract(wellFormedPair))
	if err != nil {
		t.Fatalf("extensionJobs: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want 1", len(decls))
	}
	d := decls[0]
	if d.Unit != "demo" || d.Job != "refresh" || d.Queue != "default" {
		t.Fatalf("identity came back as %+v", d)
	}
	if d.Cadence != 6*time.Hour || d.DispatcherTimeout != time.Minute || d.Timeout != 5*time.Minute || d.MaxAttempts != 3 {
		t.Fatalf("mechanics came back as %+v", d)
	}
	if d.DispatcherKind() != "ext_demo_refresh" || d.ChildKind() != "ext_demo_refresh_ws" {
		t.Fatalf("kinds came back as %s / %s", d.DispatcherKind(), d.ChildKind())
	}
}

// weekdayPair is wellFormedPair ticking on weekdays at 07:30 local time.
var weekdayPair = strings.Replace(wellFormedPair, "    cadence: 6h\n", "    at: \"07:30\"\n    days: [mon, tue, wed, thu, fri]\n", 1)

func TestExtensionJobsReadsALocalClockTimeAndItsDays(t *testing.T) {
	decls, err := extensionJobs(demoUnits(), jobsContract(weekdayPair))
	if err != nil {
		t.Fatalf("extensionJobs: %v", err)
	}
	d := decls[0]
	if d.Cadence != 0 || d.At == nil || d.At.Minute != 7*60+30 || len(d.At.Days) != 5 || d.At.Days[0] != time.Monday || d.At.Days[4] != time.Friday {
		t.Fatalf("schedule came back as cadence %s, at %+v", d.Cadence, d.At)
	}
	src := string(extensionsGen([]extensionUnit{{Name: "demo", ModulePath: "example.test/ext/demo"}}, nil, decls, nil))
	if _, err := canonicalGoSource("extensions_gen.go", []byte(src)); err != nil {
		t.Fatalf("the emitted composition is not canonical gofmt: %v\n%s", err, src)
	}
	if want := "&extension.TimeOfDay{Minute: 7*60 + 30, Days: []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}}"; !strings.Contains(src, want) {
		t.Errorf("the emitted composition misses %s:\n%s", want, src)
	}
}

// TestExtensionJobsIgnoresTheCoreKinds: a vanilla contract composes no jobs,
// which is what keeps the composed lane a superset of the vanilla one.
func TestExtensionJobsIgnoresTheCoreKinds(t *testing.T) {
	decls, err := extensionJobs(nil, jobsContract(""))
	if err != nil {
		t.Fatalf("extensionJobs: %v", err)
	}
	if len(decls) != 0 {
		t.Fatalf("a contract with no extension kinds composed %d job(s)", len(decls))
	}
}

// TestExtensionJobsRefusesTheShapesAFragmentMustNotPublish. Each case is one
// way a fragment could publish a job the running system could not honour, and
// each has to fail at generation — where the author is looking — rather than
// at a boot the author never runs.
func TestExtensionJobsRefusesTheShapesAFragmentMustNotPublish(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kinds string
		want  string
	}{
		{
			name:  "a queue the installation does not declare",
			kinds: strings.ReplaceAll(wellFormedPair, "queue: default", "queue: ext_demo_pool"),
			want:  "which the contract does not declare",
		},
		{
			name:  "a dispatcher with no child",
			kinds: strings.Split(wellFormedPair, "  ext_demo_refresh_ws:")[0],
			want:  "nothing declares ext_demo_refresh_ws",
		},
		{
			name: "a child no dispatcher fans out to",
			kinds: `
  ext_demo_orphan_ws:
    job: orphan
    role: worker
    queue: default
    timeout: 5m
    max_attempts: 3
`,
			want: "no dispatcher fans out to",
		},
		{
			name:  "a cadence on the worker kind",
			kinds: wellFormedPair + "    cadence: 1h\n",
			want:  "declares a cadence",
		},
		{
			name:  "an unrecognised key",
			kinds: wellFormedPair + "    registration: {when: [GmailRegistry]}\n",
			want:  "field registration not found",
		},
		{
			name:  "a kind whose namespace no enabled unit owns",
			kinds: strings.ReplaceAll(wellFormedPair, "ext_demo_", "ext_absent_"),
			want:  "no enabled unit owns it",
		},
		{
			name:  "a job name the kind does not spell",
			kinds: strings.ReplaceAll(wellFormedPair, "job: refresh", "job: rebuild"),
			want:  "whose kind is ext_demo_rebuild",
		},
		{
			name:  "an attempt cap on the dispatcher",
			kinds: strings.Replace(wellFormedPair, "    cadence: 6h", "    cadence: 6h\n    max_attempts: 9", 1),
			want:  "the attempt cap is the CHILD's",
		},
		{
			name:  "a split pool",
			kinds: strings.Replace(wellFormedPair, "    queue: default\n    timeout: 5m", "    queue: ai_capture\n    timeout: 5m", 1),
			want:  "share one pool",
		},
		{
			name:  "a tier no extension may request",
			kinds: strings.Replace(wellFormedPair, "tier: auto_execute", "tier: whenever", 1),
			want:  "is not one an extension may request",
		},
		{
			name:  "no cadence",
			kinds: strings.Replace(wellFormedPair, "    cadence: 6h\n", "", 1),
			want:  "declares no cadence",
		},
		{
			name:  "an unparseable duration",
			kinds: strings.Replace(wellFormedPair, "cadence: 6h", "cadence: soon", 1),
			want:  "invalid duration",
		},
		{
			// A misspelling must not be able to un-declare a job: both role
			// comparisons in the pairing are `!= X → skip`, so an unknown value
			// falls out of each and leaves the kind registered by nothing.
			name:  "a role outside the vocabulary",
			kinds: strings.Replace(wellFormedPair, "role: dispatcher", "role: dispatchr", 1),
			want:  `declares role "dispatchr"`,
		},
		{
			// The child's apparent parent is another WORKSPACE kind, so the
			// suffix-stripped lookup finds an entry and the claim check passed
			// — while the dispatcher loop, which only visits dispatchers, never
			// enqueued it.
			name:  "a worker child hanging off a worker kind",
			kinds: wellFormedPair + strings.ReplaceAll(wellFormedPair[strings.Index(wellFormedPair, "  ext_demo_refresh_ws:"):], "ext_demo_refresh_ws", "ext_demo_refresh_ws_ws"),
			want:  "no dispatcher fans out to",
		},
		{
			name:  "a cadence and a time of day",
			kinds: strings.Replace(wellFormedPair, "    cadence: 6h\n", "    cadence: 6h\n    at: \"07:30\"\n", 1),
			want:  "both cadence and at",
		},
		{
			name:  "a time past the day",
			kinds: strings.Replace(wellFormedPair, "    cadence: 6h\n", "    at: \"24:00\"\n", 1),
			want:  "want a local time as HH:MM",
		},
		{
			name:  "a time without its leading zero",
			kinds: strings.Replace(wellFormedPair, "    cadence: 6h\n", "    at: \"7:30\"\n", 1),
			want:  "want a local time as HH:MM",
		},
		{
			name:  "an unknown day",
			kinds: strings.Replace(wellFormedPair, "    cadence: 6h\n", "    at: \"07:30\"\n    days: [monday]\n", 1),
			want:  `"monday" is not one of`,
		},
		{
			name:  "a day named twice",
			kinds: strings.Replace(wellFormedPair, "    cadence: 6h\n", "    at: \"07:30\"\n    days: [mon, mon]\n", 1),
			want:  "named twice",
		},
		{
			name:  "days without a time",
			kinds: strings.Replace(wellFormedPair, "    cadence: 6h\n", "    cadence: 6h\n    days: [mon]\n", 1),
			want:  "days without at",
		},
		{
			name:  "a time of day on the worker kind",
			kinds: wellFormedPair + "    at: \"07:30\"\n",
			want:  "declares a cadence or a time of day",
		},
		{
			// Governance is the dispatcher's, and the pair folds into one
			// declaration — a second copy on the child is read by nobody.
			name:  "governance declared twice",
			kinds: wellFormedPair + "    tier: auto_execute\n    scope: read\n",
			want:  "declares tier/scope",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := extensionJobs(demoUnits(), jobsContract(tc.kinds))
			if err == nil {
				t.Fatalf("extensionJobs accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("extensionJobs(%s) = %v, want a message mentioning %q", tc.name, err, tc.want)
			}
		})
	}
}

// TestExtensionJobsAttributesAKindToTheLongestOwnedNamespace: a unit name may
// carry a hyphen, which becomes an underscore in the namespace, so `ext_a_b`
// and `ext_a_b_c` are both plausible prefixes of one kind string. Longest wins,
// or one unit would silently own the other's jobs.
func TestExtensionJobsAttributesAKindToTheLongestOwnedNamespace(t *testing.T) {
	units := []extensionUnit{{Name: "a"}, {Name: "a-b"}}
	decls, err := extensionJobs(units, jobsContract(`
  ext_a_b_refresh:
    job: refresh
    role: dispatcher
    queue: default
    timeout: 1m
    cadence: 6h
    tier: auto_execute
    scope: read
  ext_a_b_refresh_ws:
    job: refresh
    role: worker
    queue: default
    timeout: 5m
    max_attempts: 3
`))
	if err != nil {
		t.Fatalf("extensionJobs: %v", err)
	}
	if len(decls) != 1 || decls[0].Unit != "a-b" {
		t.Fatalf("got %+v, want the kind attributed to a-b", decls)
	}
}

// TestTheEmittedJobLiteralsAreCanonicalAndReadable covers the emitter arm no
// committed unit reaches yet: the vanilla tree declares no jobs, so without this
// the `time` import, the mustDuration helper and the literals themselves are
// only exercised the first time somebody adds a job — at which point a
// formatting slip fails the whole composition rather than this test.
func TestTheEmittedJobLiteralsAreCanonicalAndReadable(t *testing.T) {
	decls, err := extensionJobs(demoUnits(), jobsContract(wellFormedPair))
	if err != nil {
		t.Fatalf("extensionJobs: %v", err)
	}
	src := extensionsGen([]extensionUnit{{Name: "demo", ModulePath: "example.test/ext/demo"}}, nil, decls, nil)
	if _, err := canonicalGoSource("extensions_gen.go", src); err != nil {
		t.Fatalf("the emitted composition is not canonical gofmt: %v\n%s", err, src)
	}
	for _, want := range []string{
		`"time"`,
		"func Jobs() []extension.JobDeclaration {",
		`Unit:              "demo",`,
		`Job:               "refresh",`,
		// The point of the helper: a reviewer reads the contract's own
		// spelling, not 21600000000000.
		`Cadence:           mustDuration("6h0m0s"),`,
		`DispatcherTimeout: mustDuration("1m0s"),`,
		`Timeout:           mustDuration("5m0s"),`,
		`MaxAttempts:       3,`,
		`At:                nil,`,
		"func mustDuration(s string) time.Duration {",
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("the emitted composition misses %s:\n%s", want, src)
		}
	}
	if strings.Contains(string(src), "21600000000000") {
		t.Errorf("a raw nanosecond count reached the emitted composition:\n%s", src)
	}
}

// TestTheVanillaCompositionImportsNoTime: the helper and its import are emitted
// only where there are jobs, so the committed stub — which is compared byte for
// byte against the empty-tree output — stays what it is.
func TestTheVanillaCompositionImportsNoTime(t *testing.T) {
	src := string(extensionsGen(nil, nil, nil, nil))
	for _, unwanted := range []string{`"time"`, "mustDuration"} {
		if strings.Contains(src, unwanted) {
			t.Errorf("the vanilla composition carries %s:\n%s", unwanted, src)
		}
	}
}

// An operator resolved a job at its schedule, so moving the clock time asks
// again, while an interval job keeps the digest it was resolved under.
func TestTheJobDigestCoversTheClockTimeAndKeepsTheIntervalDigest(t *testing.T) {
	interval, err := extensionJobs(demoUnits(), jobsContract(wellFormedPair))
	if err != nil {
		t.Fatal(err)
	}
	// The digest this pair had before a job could run at a clock time.
	const before = "sha256:01ef22a3824ff8b299b7afb7404b35b093531570cb32e35769c4ad2d1b456a26"
	if got := jobDeclarationHash(interval[0]); got != before {
		t.Errorf("an interval job's digest moved: %s, want %s", got, before)
	}
	clock, err := extensionJobs(demoUnits(), jobsContract(weekdayPair))
	if err != nil {
		t.Fatal(err)
	}
	later := clock[0]
	later.At = &extension.TimeOfDay{Minute: clock[0].At.Minute + 60, Days: clock[0].At.Days}
	if jobDeclarationHash(clock[0]) == jobDeclarationHash(later) {
		t.Error("moving the clock time left the digest unchanged")
	}
}
