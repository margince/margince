// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// No shipped UI string promises that nothing sends without a human's approval
// while the generated policy table says the send verbs auto-execute.
//
// This is the defect it exists for, and it shipped. The product moved sending
// from confirm-first to auto-execute for a good reason — a person's grant of the
// `send` scope IS the approval, so asking them to confirm each message made the
// agent weaker than the person behind it — and the copy did not move with it.
// Three English strings, plus their German and Vietnamese translations, went on
// telling users that nothing leaves the building without their say-so. Every
// other gate was green: the strings were valid, the screens rendered, the
// translations were complete. A promise about safety that the server does not
// keep is worse than a missing feature, and nothing in the build could see it.
//
// The obligation is DERIVED, not pinned. It reads the tier the generator wrote
// for the send verbs, and only forbids the absolute claim while that tier is
// auto-execute. An installation-wide decision to floor sending back to
// confirm-first would make the old copy true again, and this gate would step
// aside rather than having to be edited — which is the difference between a
// fitness function and a second copy of the answer.
//
// WHAT IT CANNOT SEE. It matches phrases, so a fourth way of saying the same
// thing walks past it, and it reads only the locale files. That is the honest
// limit of a prose check, and the reason the phrases below are shapes rather
// than whole sentences. It is a backstop for one known-expensive claim, not a
// proof that the copy is true.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// sendTools are the tool verbs whose tier decides whether the absolute claim is
// a lie: the two that put a message outside the installation.
var sendTools = []string{"send_email", "send_message"}

// contractTiers are the only tiers the contract admits. A tier outside this set
// means the vocabulary moved under this gate, which must stop it rather than
// read the unknown word as "not auto-execute".
var contractTiers = map[string]bool{
	"auto_execute":          true,
	"confirmation_required": true,
	"dynamic":               true,
}

// absoluteApprovalClaims are the shapes of "we never send without your approval".
//
// Written lowercase and without punctuation so a comma or a capital does not
// smuggle one past, and kept to the CLAIM itself: "waits for your approval" is
// fine beside a verb that does wait, so a phrase here has to carry the absolute
// ("never", "nothing", "anything that leaves") that makes it false.
var absoluteApprovalClaims = map[string][]*regexp.Regexp{
	"en": {
		regexp.MustCompile(`never sends? (anything|any message|an email|a message)`),
		regexp.MustCompile(`without asking you first`),
		regexp.MustCompile(`anything that leaves the building waits`),
		regexp.MustCompile(`nothing (is sent|sends|leaves) (without|until)`),
		regexp.MustCompile(`sends? only what you approve`),
		regexp.MustCompile(`(is |are )?(ever )?sent without your approval`),
	},
	"de": {
		regexp.MustCompile(`(senden|sende|verschickt) nie (etwas|eine e-mail|eine nachricht)`),
		regexp.MustCompile(`ohne dich vorher zu fragen`),
		regexp.MustCompile(`nichts wird ohne .{0,20}freigabe`),
		regexp.MustCompile(`sendet nur, was du freigibst`),
		regexp.MustCompile(`ohne deine freigabe geht (nie )?(et)?was raus`),
		regexp.MustCompile(`nichts wird gesendet, bevor du`),
	},
	"vi": {
		regexp.MustCompile(`(không gì được gửi|không (bao giờ )?(tự )?gửi gì)[^.]{0,40}duyệt`),
		regexp.MustCompile(`không bao giờ gửi (email|tin nhắn)`),
		regexp.MustCompile(`mà chưa hỏi bạn trước`),
		regexp.MustCompile(`chỉ gửi những gì bạn duyệt`),
	},
}

// autoExecutingSendVerbs names the send verbs that run without staging an
// approval, read from the COMPILED policy table.
//
// It asks compose.AgentToolTiers() rather than grepping agentpolicy_gen.go,
// because a text scrape fails short in the one direction that is silent. An
// earlier version matched the literal `Tier: "auto_execute"` on the generated
// line; anything changing that spelling — gofmt padding, a multi-line literal, a
// renamed token — dropped both verbs out of the result, and the gate then logged
// that the claim was true and read not one locale string. That happened during
// review, on a tree where both verbs were live at auto_execute.
//
// ANY auto-executing verb is enough to make the absolute claim false, so this
// returns the list rather than one bool over all of them: flooring send_email
// while send_message still executes must not switch the whole check off.
func autoExecutingSendVerbs(t *testing.T) []string {
	t.Helper()
	declared := compose.AgentToolTiers()
	if len(declared) == 0 {
		t.Fatal("the contract declares no tool tiers — this gate is reading a contract shape that is gone")
	}

	var auto []string
	for _, tool := range sendTools {
		tiers, governed := declared[tool]
		if !governed {
			t.Fatalf("the contract governs no tool named %q — a send verb was renamed or removed, and this gate must be re-read against that change rather than quietly checking one verb less", tool)
		}
		for _, tier := range tiers {
			if !contractTiers[tier] {
				t.Fatalf("%s resolves to tier %q, which the contract does not admit — the tier vocabulary moved under this gate", tool, tier)
			}
			if tier == "auto_execute" {
				auto = append(auto, tool)
				break
			}
		}
	}
	return auto
}

// shippedLocales names every locale the app ships, read from the directory
// rather than from the map below.
//
// This is the half that keeps the gate from failing short. The claim patterns
// are per-LANGUAGE and cannot be derived — somebody has to write down how the
// promise is spelled in French before French can be checked — so a hand-written
// map is unavoidable here. What is avoidable is that map going quietly short: a
// new locale file with no patterns beside it would be a whole language this gate
// reads nothing of, and it would report PASS. So the directory decides who must
// be covered, and an uncovered locale FAILS rather than being skipped.
func shippedLocales(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the locale directory: %v", err)
	}
	// The machinery that lives beside the catalogs, named as the exclusions they
	// are rather than filtered by the SHAPE of a locale code. A length test for
	// "two characters" silently drops pt-BR and zh-Hans: a whole language read
	// by nothing, while the gate reports PASS. That is the same
	// under-recognition this file exists to refuse, one level up.
	machinery := map[string]bool{"index.ts": true, "publiclocale.ts": true}

	var locales []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".ts") || strings.Contains(name, ".test.") || machinery[name] {
			continue
		}
		locales = append(locales, strings.TrimSuffix(name, ".ts"))
	}
	if len(locales) == 0 {
		t.Fatal("no locale files found — this gate is reading a tree shape that is gone")
	}
	return locales
}

// shippedCopy drops whole-line comments, leaving what a user can actually read.
//
// A locale file explains its own copy, and this gate went off on a comment
// describing the very claim it had just removed. Left alone, that teaches the
// next author to weaken the patterns to quiet it — trading a real matcher for a
// tidy log — so the fix belongs on the CORPUS instead: judge shipped strings,
// never the prose about them.
//
// Only lines that BEGIN with the marker go, so a "https://…" inside a string
// keeps its slashes. A trailing comment after a string survives, and is
// harmless: it is one line of prose on a line that also carries real copy.
func shippedCopy(doc string) string {
	var kept []string
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func TestNoLocaleClaimsSendingAlwaysWaitsForAHuman(t *testing.T) {
	t.Parallel()

	// Not t.Skip: a skipped gate and a passing one read the same in a log, and
	// this one guards a claim about safety. Say out loud that the obligation
	// lifted, and why.
	autoExecuting := autoExecutingSendVerbs(t)
	if len(autoExecuting) == 0 {
		t.Log("every send verb stages an approval, so the absolute claim is true and this gate holds nothing today")
		return
	}
	t.Logf("send verbs running at auto_execute: %s", strings.Join(autoExecuting, ", "))

	dir := filepath.Join(repoRoot, "frontend", "src", "i18n")
	for _, locale := range shippedLocales(t, dir) {
		claims, covered := absoluteApprovalClaims[locale]
		if !covered {
			t.Errorf("frontend/src/i18n/%s.ts ships to users but this gate knows no %s phrasing of \"nothing sends without your approval\", so it reads none of that language. Add the shapes to absoluteApprovalClaims — an uncovered locale is where this census goes short", locale, locale)
			continue
		}

		raw, err := os.ReadFile(filepath.Join(dir, locale+".ts"))
		if err != nil {
			t.Fatalf("reading the %s locale: %v", locale, err)
		}
		body := strings.ToLower(shippedCopy(string(raw)))

		for _, claim := range claims {
			if loc := claim.FindStringIndex(body); loc != nil {
				t.Errorf("frontend/src/i18n/%s.ts promises %q, but the contract runs the send verbs at auto_execute — the app is telling a user their approval gates something it does not gate", locale, strings.TrimSpace(body[loc[0]:loc[1]]))
			}
		}
	}
}
