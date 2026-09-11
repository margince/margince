// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The distinction the surface was missing.
//
// An assistant asked to assign work searched `person` — the customer contacts
// — found two people with the right first name, and reported that neither was
// "listed under sales". The seat it wanted was the one the human was signed in
// as, and no tool could name it. assignee_id and owner_id take these ids.
func TestListColleaguesAnswersSeatsWithWhatAssignmentNeeds(t *testing.T) {
	lena := ids.NewV7()
	out, err := listColleagues{list: func(_ context.Context, q string) ([]Colleague, bool, error) {
		if q != "lena" {
			t.Errorf("the filter reached the roster as %q, want it forwarded", q)
		}
		return []Colleague{{
			UserID: lena, DisplayName: "Lena Fischer", Email: "lena.fischer@demo.test",
			SeatType: "full",
		}}, false, nil
	}}.Handle(context.Background(), json.RawMessage(`{"q":"lena"}`))
	if err != nil {
		t.Fatalf("listing colleagues answered %v, want the roster", err)
	}
	var got ListColleaguesResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got.Colleagues) != 1 || got.Colleagues[0].UserID != lena {
		t.Fatalf("roster = %+v, want the seat that id names", got.Colleagues)
	}
	if got.Truncated {
		t.Error("a one-seat roster reports truncated, want it complete")
	}
}

// A filter matching nobody is an answer, not an error, and every list in it
// serializes as an empty array rather than null — a caller iterating the result
// must not have to guard for both.
func TestListColleaguesAnswersAnEmptyRosterAsAList(t *testing.T) {
	out, err := listColleagues{list: func(context.Context, string) ([]Colleague, bool, error) {
		return nil, false, nil
	}}.Handle(context.Background(), json.RawMessage(`{"q":"nobody"}`))
	if err != nil {
		t.Fatalf("answered %v, want an empty roster", err)
	}
	// `all_colleagues` rides along because a narrowing that matched nobody is
	// answered with the set it was matched against, and here that set is empty
	// too — which the wire says as `[]` rather than by leaving the field out.
	if got := string(out); got != `{"colleagues":[],"all_colleagues":[]}` {
		t.Errorf("empty roster serialized as %s, want empty arrays", got)
	}
}

// A capped roster says so. Told nothing, a caller reads 200 seats as the whole
// company and reports that a colleague does not work here.
func TestListColleaguesSaysWhenTheRosterIsLongerThanTheAnswer(t *testing.T) {
	out, err := listColleagues{list: func(context.Context, string) ([]Colleague, bool, error) {
		return []Colleague{{UserID: ids.NewV7(), DisplayName: "A"}}, true, nil
	}}.Handle(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("answered %v, want a roster", err)
	}
	var got ListColleaguesResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if !got.Truncated {
		t.Error("a capped roster did not report truncated")
	}
}

// An empty list answers two different questions identically: this workspace
// employs nobody, and nobody here is spelled the way you asked. A caller cannot
// tell them apart, and the one it picks is the wrong one — asked to hand an
// account to a colleague, an assistant read the empty list and reported that
// the person does not work here, with the seat sitting in the roster under a
// spelling it had not tried.
func TestAColleagueMissHandsOverTheRosterItWasMatchedAgainst(t *testing.T) {
	t.Parallel()
	lena, tom := ids.NewV7(), ids.NewV7()
	asked := []string{}
	out, err := listColleagues{list: func(_ context.Context, q string) ([]Colleague, bool, error) {
		asked = append(asked, q)
		if q != "" {
			return nil, false, nil
		}
		return []Colleague{
			{UserID: lena, DisplayName: "Lena Fischer", Email: "lena.fischer@demo.test"},
			{UserID: tom, DisplayName: "Tom Brand", Email: "tom@demo.test"},
		}, false, nil
	}}.Handle(context.Background(), json.RawMessage(`{"q":"Fischer, Lena"}`))
	if err != nil {
		t.Fatalf("answered %v, want a roster", err)
	}

	var got ListColleaguesResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got.Colleagues) != 0 {
		t.Errorf("the narrowed answer is not empty (%+v) — it must stay the honest answer to "+
			"what was asked", got.Colleagues)
	}
	if got.AllColleagues == nil || len(*got.AllColleagues) != 2 {
		t.Fatalf("the miss handed over %d seats, want the 2 it was matched against — a caller "+
			"told only \"no match\" reports that the person does not work here", len(*got.AllColleagues))
	}
	if (*got.AllColleagues)[0].UserID != lena {
		t.Errorf("the roster is not the seats the lister answered: %+v", *got.AllColleagues)
	}
	if want := []string{"Fischer, Lena", ""}; !slices.Equal(asked, want) {
		t.Errorf("the lister was asked %v, want %v — the roster read happens ONLY on the miss", asked, want)
	}
}

// And not on a hit: a narrowing that found somebody has no second question to
// answer, and reading the whole roster to attach it would cost every successful
// call an extra query.
func TestAColleagueHitDoesNotAlsoReadTheWholeRoster(t *testing.T) {
	t.Parallel()
	reads := 0
	out, err := listColleagues{list: func(_ context.Context, q string) ([]Colleague, bool, error) {
		reads++
		return []Colleague{{UserID: ids.NewV7(), DisplayName: "Lena Fischer"}}, false, nil
	}}.Handle(context.Background(), json.RawMessage(`{"q":"Lena"}`))
	if err != nil {
		t.Fatalf("answered %v, want a roster", err)
	}
	if reads != 1 {
		t.Errorf("the lister was read %d times for a query that matched, want 1", reads)
	}
	var got ListColleaguesResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if got.AllColleagues != nil {
		t.Errorf("a matching query also carried the whole roster: %+v", got.AllColleagues)
	}
}

// A roster read with no `q` has nothing to fall back to, and must not recurse
// into itself looking for one.
func TestAnEmptyWorkspaceAnswersEmptyWithoutAskingTwice(t *testing.T) {
	t.Parallel()
	reads := 0
	out, err := listColleagues{list: func(context.Context, string) ([]Colleague, bool, error) {
		reads++
		return nil, false, nil
	}}.Handle(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("answered %v, want an empty roster", err)
	}
	if reads != 1 {
		t.Errorf("an unnarrowed empty roster was read %d times, want 1", reads)
	}
	if got := string(out); got != `{"colleagues":[]}` {
		t.Errorf("an empty workspace answered %s", got)
	}
}

// colleagueMiss drives one narrowing that finds nobody, with an envelope around
// it so the warnings the miss raises are readable.
func colleagueMiss(t *testing.T, q string, list ColleagueLister) (ListColleaguesResult, *envelopeFacts) {
	t.Helper()
	ctx, facts := withEnvelopeFacts(context.Background())
	out, err := listColleagues{list: list}.Handle(ctx, json.RawMessage(`{"q":"`+q+`"}`))
	if err != nil {
		t.Fatalf("answered %v, want an answer", err)
	}
	var got ListColleaguesResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	return got, facts
}

func warnedWith(facts *envelopeFacts, code string) (Warning, bool) {
	for _, w := range facts.warnings {
		if w.Code == code {
			return w, true
		}
	}
	return Warning{}, false
}

// A CAPPED fallback is the case this whole affordance can get wrong. Two
// hundred alphabetical names with the one asked for absent reads as proof the
// person has no seat — a caller MORE certain of the wrong answer than a bare
// empty list left it. So the cap rides its own flag, and the warning says do
// not conclude absence from it.
func TestACappedFallbackSaysItIsAPageAndNotTheWorkforce(t *testing.T) {
	t.Parallel()
	got, facts := colleagueMiss(t, "Fischer", func(_ context.Context, q string) ([]Colleague, bool, error) {
		if q != "" {
			return nil, false, nil
		}
		return []Colleague{{UserID: ids.NewV7(), DisplayName: "Aaron Adler"}}, true, nil
	})

	if got.Truncated {
		t.Error("the query's own flag says the NARROWING has more matches — it matched nothing")
	}
	if !got.AllColleaguesTruncated {
		t.Error("a capped fallback does not say it is capped, so a caller reads a page as the workforce")
	}
	warning, ok := warnedWith(facts, CodeNameMatchedNoColleague)
	if !ok {
		t.Fatalf("the miss raised no warning: %+v", facts.warnings)
	}
	if !strings.Contains(warning.Message, "CAPPED") {
		t.Errorf("the warning does not say the list is a page:\n%s", warning.Message)
	}
}

// The question that was asked has already been answered correctly by the time
// the fallback runs. A courtesy read that fails must not turn that into a tool
// failure — the shape reportDuplicates settled for the same situation — and an
// absent fallback must read as unknown rather than as an empty workspace.
func TestAnUnreadableFallbackDoesNotDestroyTheAnswerItWasAddedTo(t *testing.T) {
	t.Parallel()
	got, facts := colleagueMiss(t, "Fischer", func(_ context.Context, q string) ([]Colleague, bool, error) {
		if q != "" {
			return nil, false, nil
		}
		return nil, false, errors.New("the roster read failed")
	})

	if got.AllColleagues != nil {
		t.Errorf("a failed fallback still attached a list: %+v", *got.AllColleagues)
	}
	warning, ok := warnedWith(facts, CodeRosterUnreadable)
	if !ok {
		t.Fatalf("a failed fallback is silent, so an absent list reads as an empty workspace: %+v",
			facts.warnings)
	}
	if !strings.Contains(warning.Message, "unknown") {
		t.Errorf("the warning does not say the answer is unknown:\n%s", warning.Message)
	}
}

// A workspace that really employs nobody says THAT, rather than leaving the
// caller to read an absent list.
func TestAWorkspaceWithNoSeatsSaysSoRatherThanGoingQuiet(t *testing.T) {
	t.Parallel()
	_, facts := colleagueMiss(t, "Fischer", func(context.Context, string) ([]Colleague, bool, error) {
		return nil, false, nil
	})
	warning, ok := warnedWith(facts, CodeNameMatchedNoColleague)
	if !ok {
		t.Fatalf("an empty workspace raised no warning: %+v", facts.warnings)
	}
	if !strings.Contains(warning.Message, "no colleagues") {
		t.Errorf("the warning does not say the workspace has nobody:\n%s", warning.Message)
	}
}

// Whitespace is not a narrowing. The roster read trims before deciding, so a
// handler that did not would call it twice for a `q` the service treats as
// absent — one invariant with two definitions across a seam.
func TestWhitespaceIsNotANarrowingOnEitherSideOfTheSeam(t *testing.T) {
	t.Parallel()
	reads := 0
	ctx, _ := withEnvelopeFacts(context.Background())
	if _, err := (listColleagues{list: func(context.Context, string) ([]Colleague, bool, error) {
		reads++
		return nil, false, nil
	}}).Handle(ctx, json.RawMessage(`{"q":"   "}`)); err != nil {
		t.Fatalf("answered %v", err)
	}
	if reads != 1 {
		t.Errorf("a whitespace `q` was read %d times, want 1 — the handler and the roster read "+
			"disagree about what counts as narrowing", reads)
	}
}

// The two fallback outcomes must differ in the BYTES, not only in a warning.
//
// A workspace that employs nobody and a roster that could not be read are
// different answers, and a caller acting on `data` alone saw the same thing
// from both — the omitted field. That is the defect this whole field exists to
// remove, one level up.
func TestAnEmptyFallbackAndAnUnreadableOneAreDifferentOnTheWire(t *testing.T) {
	t.Parallel()
	ctx, _ := withEnvelopeFacts(context.Background())

	empty, err := listColleagues{list: func(context.Context, string) ([]Colleague, bool, error) {
		return nil, false, nil
	}}.Handle(ctx, json.RawMessage(`{"q":"Fischer"}`))
	if err != nil {
		t.Fatalf("answered %v", err)
	}
	if !strings.Contains(string(empty), `"all_colleagues":[]`) {
		t.Errorf("a workspace that employs nobody does not say so in the data:\n%s", empty)
	}

	unreadable, err := listColleagues{list: func(_ context.Context, q string) ([]Colleague, bool, error) {
		if q == "" {
			return nil, false, errors.New("the roster read failed")
		}
		return nil, false, nil
	}}.Handle(ctx, json.RawMessage(`{"q":"Fischer"}`))
	if err != nil {
		t.Fatalf("answered %v", err)
	}
	if strings.Contains(string(unreadable), "all_colleagues") {
		t.Errorf("an unreadable roster claims a list it never read:\n%s", unreadable)
	}
	if string(empty) == string(unreadable) {
		t.Errorf("both fallback outcomes serialize identically, so a caller reading the data "+
			"cannot tell an empty workspace from one it could not see:\n%s", empty)
	}
}
