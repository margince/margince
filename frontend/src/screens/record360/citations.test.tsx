/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { Citations, type Cited, citationChips } from "./citations";

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
