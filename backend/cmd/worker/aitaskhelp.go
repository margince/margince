// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The `worker aitask` command tree: which verbs exist, what each one is for,
// and the help every one of them prints.
//
// ONE catalog, read by three callers — the dispatcher, the refusal for a verb
// nobody serves, and the help itself. A tool whose help is a second list of its
// verbs advertises the one somebody forgot to delete and hides the one they
// forgot to add, and an operator has no way to tell which list is the truth.
//
// A verb's FLAGS are deliberately not written down here. Each entry hands back
// the flag set its parser really builds, so `aitask <verb> --help` prints what
// the parser accepts rather than what a usage block last remembered about it.

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// aiTaskVerb is one verb: its name, the one line that says why you would reach
// for it, its argument shape, and its parser's flag set (nil for a verb that
// takes none).
type aiTaskVerb struct {
	name    string
	summary string
	usage   string
	flags   func() *flag.FlagSet
}

// aiTaskVerbs is the catalog. The order is the order an operator meets them:
// what exists, what a fixture looks like, the two ways to produce one, the verb
// that spends the most, and the help.
func aiTaskVerbs() []aiTaskVerb {
	probeFlags := func(verb string) func() *flag.FlagSet {
		return func() *flag.FlagSet {
			fs, _, _ := aiTaskFlagSet(verb)
			return fs
		}
	}
	return []aiTaskVerb{
		{
			verbList, "name every invocation site this build ships, and whether the corpus already carries a scenario for it",
			"list", probeFlags(verbList),
		},
		{
			verbScaffold, "print a runnable starter scenario for one site, copied from the corpus — how a fixture's shape is discovered",
			"scaffold <task>/<variant>", probeFlags(verbScaffold),
		},
		{
			verbFetch, "fetch a URL through the production reader and keep the reduced text an extraction site is handed",
			"fetch <url>", probeFlags(verbFetch),
		},
		{
			verbRetrieve, "rank the shipped handbook for one question, with the production chunker and a real embed lane, and capture what a run would be handed",
			`retrieve --question "..." --model <provider:model> [--page records.md]`, func() *flag.FlagSet {
				fs, _, _ := aiTaskRetrieveFlagSet()
				return fs
			},
		},
		{
			verbRun, "drive one site's certification case over input you supply — the only verb that calls a model",
			"run --site <task>/<variant> --fixture <file> --expect <file> --model <provider:model>", probeFlags(verbRun),
		},
		{verbHelp, "print this, or one verb's own flags", "help [verb]", nil},
	}
}

func lookupAITaskVerb(name string) (aiTaskVerb, bool) {
	for _, v := range aiTaskVerbs() {
		if v.name == name {
			return v, true
		}
	}
	return aiTaskVerb{}, false
}

func knownAITaskVerb(name string) bool {
	_, ok := lookupAITaskVerb(name)
	return ok
}

// aiTaskVerbNames names every verb, for the messages that have to list them.
func aiTaskVerbNames() []string {
	verbs := aiTaskVerbs()
	names := make([]string, 0, len(verbs))
	for _, v := range verbs {
		names = append(names, v.name)
	}
	return names
}

// runAITaskHelp serves `aitask help [verb]`.
//
// Its argument is a VERB, never a flag: `aitask help --help` is somebody asking
// for help about help, and printing the tree is the only answer to that which
// is not a joke at their expense.
func runAITaskHelp(w io.Writer, args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return writeAITaskOverview(w)
	}
	verb, known := lookupAITaskVerb(args[0])
	if !known {
		return unknownAITaskVerb(w, args[0])
	}
	return writeAITaskVerbHelp(w, verb)
}

// unknownAITaskVerb prints the whole tree and then refuses, so the operator who
// mistyped reads the list rather than a sentence telling them one exists.
func unknownAITaskVerb(w io.Writer, verb string) error {
	if err := writeAITaskOverview(w); err != nil {
		return err
	}
	return fmt.Errorf("aitask: unknown verb %q — the verbs are listed above", verb)
}

// helpOnRequest turns a parser's `-h` into that verb's own help and leaves every
// other outcome alone. `flag` reports the request as ErrHelp AFTER it has
// distinguished flags from their values, which is why the request is recognised
// here and not by scanning the arguments for "-h" — a question whose text is
// "-h" is silly, but answering it with a usage block is a bug.
func helpOnRequest(w io.Writer, verb string, err error) error {
	if !errors.Is(err, flag.ErrHelp) {
		return err
	}
	v, known := lookupAITaskVerb(verb)
	if !known {
		return writeAITaskOverview(w)
	}
	return writeAITaskVerbHelp(w, v)
}

const aiTaskPreamble = `worker aitask — probe one AI invocation site against input you supply.

Certification (make e2e-ai) asks whether a model is good enough for a prompt.
This asks whether a site survives THIS input, which is how a site can be
certified and still fail in the field.

Usage:
  worker aitask <verb> [flags]

Verbs:
`

const aiTaskEpilogue = `
  worker aitask <verb> --help   the flags that verb takes

Nothing here opens a database. list, scaffold and fetch are free; run calls a
model, and retrieve embeds — once per chunk, then never again for prose that has
not changed. Artifacts land in the gitignored work directory, because they carry
whatever the probed source carried.
`

func writeAITaskOverview(w io.Writer) error {
	if _, err := fmt.Fprint(w, aiTaskPreamble); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, v := range aiTaskVerbs() {
		if _, err := fmt.Fprintf(tw, "  %s\t%s\n", v.name, v.summary); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprint(w, aiTaskEpilogue)
	return err
}

func writeAITaskVerbHelp(w io.Writer, verb aiTaskVerb) error {
	if _, err := fmt.Fprintf(w, "worker aitask %s — %s\n\nUsage:\n  worker aitask %s\n",
		verb.name, verb.summary, verb.usage); err != nil {
		return err
	}
	if verb.flags == nil {
		return nil
	}
	if _, err := fmt.Fprint(w, "\nFlags:\n"); err != nil {
		return err
	}
	fs := verb.flags()
	fs.SetOutput(w)
	fs.PrintDefaults()
	return nil
}
