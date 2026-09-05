/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import {
  type BriefSentence,
  Citations,
  type Cited,
  citationChips,
  SentenceList,
} from "./citations";

// A citation names the record it rests on. The kind is what it falls back to,
// never what it prefers: a verdict citing an emailed promise rendered the bare
// word "activity" — the same label for every activity on the page — because the
// name was read only on the branch a reader could click.

function cited(entityType: Cited["entity_type"], id: string, name?: string) {
  return { entity_type: entityType, entity_id: id, name } satisfies Cited;
}

function renderCitations(evidence: Cited[], openable: boolean) {
  render(
    <LocaleProvider initial="en">
      <Citations
        evidence={evidence}
        onOpenRecord={openable ? vi.fn() : undefined}
      />
    </LocaleProvider>,
  );
}

afterEach(() => {
  cleanup();
});

describe("citationChips", () => {
  it("carries the name onto a chip the reader cannot open", () => {
    // `activity` has no detail route, so it is never openable — which is
    // exactly why the name has to reach it.
    expect(
      citationChips(
        [cited("activity", "a-1", "Slots for the pilot review")],
        () => false,
      ),
    ).toEqual([
      {
        openable: false,
        entityType: "activity",
        count: 1,
        name: "Slots for the pilot review",
      },
    ]);
  });

  it("drops the name once a chip stands for several records", () => {
    // One member's name over three activities claims the other two are that
    // thread as well. The count is the only honest label left.
    expect(
      citationChips(
        [
          cited("activity", "a-1", "Slots for the pilot review"),
          cited("activity", "a-2", "Contract questions"),
          cited("activity", "a-3"),
        ],
        () => false,
      ),
    ).toEqual([
      { openable: false, entityType: "activity", count: 3, name: undefined },
    ]);
  });

  it("counts a record cited twice once", () => {
    expect(
      citationChips(
        [cited("activity", "a-1", "Renewal"), cited("activity", "a-1")],
        () => false,
      ),
    ).toEqual([
      { openable: false, entityType: "activity", count: 1, name: "Renewal" },
    ]);
  });

  it("takes the name from whichever citation of that record carried one", () => {
    // The reverse order of the case above. The name rides the CITATION, not
    // the record, so nothing promises the first mention is the one that has
    // it — and dropping the repeat outright threw away the only name there
    // was, leaving the chip to render its bare kind.
    expect(
      citationChips(
        [cited("activity", "a-1"), cited("activity", "a-1", "Renewal")],
        () => false,
      ),
    ).toEqual([
      { openable: false, entityType: "activity", count: 1, name: "Renewal" },
    ]);
  });

  it("still refuses one member's name once the chip counts several", () => {
    // A late name must not reopen the group's mouth: the chip speaks for two
    // records here, and either name would claim the other is it.
    expect(
      citationChips(
        [
          cited("activity", "a-1"),
          cited("activity", "a-2"),
          cited("activity", "a-1", "Renewal"),
        ],
        () => false,
      ),
    ).toEqual([
      { openable: false, entityType: "activity", count: 2, name: undefined },
    ]);
  });
});

describe("Citations", () => {
  it("names an unopenable record instead of its kind", () => {
    renderCitations(
      [cited("activity", "a-1", "Slots for the pilot review")],
      true,
    );
    expect(screen.getByText("Slots for the pilot review")).toBeTruthy();
    expect(screen.queryByText("activity")).toBeNull();
  });

  it("falls back to the kind when the server sent no name", () => {
    // Nothing here invents a label: an unnamed record still says what kind of
    // record it is, which is what the reader had before.
    renderCitations([cited("activity", "a-1")], true);
    expect(screen.getByText("activity")).toBeTruthy();
  });

  it("keeps a grouped chip's count rather than one member's name", () => {
    renderCitations(
      [
        cited("activity", "a-1", "Slots for the pilot review"),
        cited("activity", "a-2", "Contract questions"),
      ],
      true,
    );
    expect(screen.getByText("2 activities")).toBeTruthy();
    expect(screen.queryByText("Slots for the pilot review")).toBeNull();
  });

  it("still names an openable record, and opens it", () => {
    // The behaviour the flat branch was missing has always held here; it is
    // pinned so the shared label cannot regress one side while fixing the
    // other.
    renderCitations([cited("deal", "d-1", "Fleet renewal 2027")], true);
    expect(
      screen.getByRole("button", { name: "Fleet renewal 2027" }),
    ).toBeTruthy();
  });

  it("renders a citation flat when the page cannot open anything", () => {
    // No onOpenRecord: every chip is unopenable, and a name is still a name.
    renderCitations([cited("deal", "d-1", "Fleet renewal 2027")], false);
    expect(screen.getByText("Fleet renewal 2027")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("groups several receipts of one kind under a count", () => {
    // fact/profile_field are openable AND grouped: the chip opens the first and
    // the drawer's stepper reaches the rest, so the count must survive.
    renderCitations(
      [cited("fact", "f-1", "Headcount"), cited("fact", "f-2", "Revenue")],
      true,
    );
    expect(screen.getByRole("button", { name: "2 facts" })).toBeTruthy();
  });
});

describe("SentenceList leading claim", () => {
  function sentence(text: string, nature: BriefSentence["nature"]) {
    return { text, nature, evidence: [] } satisfies BriefSentence;
  }

  function renderList(sentences: BriefSentence[]) {
    render(
      <LocaleProvider initial="en">
        <SentenceList sentences={sentences} leadWithJudgement />
      </LocaleProvider>,
    );
  }

  it("puts the judgement first, whatever order it was written in", () => {
    // The facts are already on the cards above. What the block adds is what
    // Margince makes of them, so that is the sentence a scanner must meet.
    renderList([
      sentence("Two deals are open.", "fact"),
      sentence("The account is drifting.", "assessment"),
      sentence("Call them this week.", "recommendation"),
    ]);
    const lead = screen.getByText(/The account is drifting/);
    expect(lead.className).toContain("co-brief-lead");
    // Promoted, never duplicated: the list under it holds the other two.
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
  });

  it("leads with the first line when nothing judges", () => {
    // The deterministic fallback writes no assessments. An empty lead slot
    // would read as a sentence that failed to load.
    renderList([
      sentence("Two deals are open.", "fact"),
      sentence("They pay on time.", "fact"),
    ]);
    expect(screen.getByText(/Two deals are open/).className).toContain(
      "co-brief-lead",
    );
    expect(screen.getAllByRole("listitem")).toHaveLength(1);
  });
});

// A chip is the reader's receipt, and a receipt that only repeats the chip's
// kind is no receipt. Where the server sent the record's own words, the date
// they are dated and where they came from, resting on the chip opens exactly
// that — and never the kind word again.
describe("a citation's receipt", () => {
  const receipted: Cited = {
    ...cited("activity", "a-1", "Slots for the pilot review"),
    quote: "Two slots next week would work on our side.",
    at: "2026-05-01T09:00:00Z",
    origin: "Email you sent",
  };

  it("opens the record's own words and where they came from", async () => {
    const user = userEvent.setup();
    renderCitations([receipted], false);

    await user.click(
      screen.getByRole("button", { name: "Slots for the pilot review" }),
    );
    expect(
      screen.getByText("Two slots next week would work on our side."),
    ).toBeTruthy();
    // The origin and the date on one line, the date in the record's own
    // calendar rather than as the wire's instant.
    expect(screen.getByText("Email you sent · 01/05/2026")).toBeTruthy();
    // Nowhere to go: an activity has no page of its own, and a receipt that
    // offered one would be a button that does nothing.
    expect(
      screen.queryByRole("button", { name: "Open the record" }),
    ).toBeNull();
  });

  it("never folds a receipted citation into a count", () => {
    // A count cannot quote: a chip for two activities that opened one
    // message's words would claim the other said the same.
    renderCitations(
      [
        receipted,
        {
          ...cited("activity", "a-2", "Contract questions"),
          origin: "Email you sent",
        },
      ],
      false,
    );
    expect(
      screen.getByRole("button", { name: "Slots for the pilot review" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Contract questions" }),
    ).toBeTruthy();
    expect(screen.queryByText("2 activities")).toBeNull();
  });

  it("offers the record from the receipt when it has a page", async () => {
    const user = userEvent.setup();
    const open = vi.fn();
    render(
      <LocaleProvider initial="en">
        <Citations
          evidence={[
            {
              ...cited("deal", "d-1", "Fleet renewal 2027"),
              at: "2026-03-14T09:00:00Z",
              origin: "Open deal, last worked",
            },
          ]}
          onOpenRecord={open}
        />
      </LocaleProvider>,
    );

    // The chip's own click opens the receipt, not the deal: a reader who
    // rested on it to check the claim must not be carried off the page.
    await user.click(
      screen.getByRole("button", { name: "Fleet renewal 2027" }),
    );
    expect(open).not.toHaveBeenCalled();
    expect(
      screen.getByText("Open deal, last worked · 14/03/2026"),
    ).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Open the record" }));
    expect(open).toHaveBeenCalledWith("deal", "d-1", []);
  });
});
