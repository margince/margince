// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { offerResearchOnOverview } from "./companies";

// Taken from the function under test rather than named again from the schema:
// the 360's own type is what this rule reads, and a second spelling of it here
// could drift from the one the rule actually sees.
type Company360View = NonNullable<
  Parameters<typeof offerResearchOnOverview>[0]["view"]
>;

// An account as the 360 reports it, with every section the mount rule reads
// present and empty. Each case below names only what it changes.
function view(over: Partial<Company360View> = {}): Company360View {
  return {
    contacts: { data: [] },
    deals: { data: [] },
    activities: { data: [] },
    sections_omitted: [],
    ...over,
  } as Company360View;
}

const bare = { overlay: false, view: view(), read: false };

describe("offerResearchOnOverview", () => {
  it("leads the column on an untouched account nobody has researched", () => {
    expect(offerResearchOnOverview(bare)).toBe(true);
  });

  it("stands down once the account has been read, however that read went", () => {
    // The case this rule was written for. A read that hit its page cap, or one
    // that failed outright, has still ANSWERED "shall we go and look" — and
    // the offer at the top of an account with staged facts behind it tells a
    // rep nobody has looked.
    expect(offerResearchOnOverview({ ...bare, read: true })).toBe(false);
  });

  it("stands down on an account somebody has already started on", () => {
    expect(
      offerResearchOnOverview({
        ...bare,
        view: view({ contacts: { data: [{}] } } as Partial<Company360View>),
      }),
    ).toBe(false);
  });

  it("never leads in overlay, where the research verb does not exist", () => {
    expect(offerResearchOnOverview({ ...bare, overlay: true })).toBe(false);
  });

  it("treats a section withheld from this reader as not empty", () => {
    // A section the reader may not see is not a section with nothing in it.
    // Offering a crawl of an account that is already full, to the one reader
    // who cannot see that it is, is the failure this guards.
    expect(
      offerResearchOnOverview({
        ...bare,
        view: view({ sections_omitted: ["contacts"] }),
      }),
    ).toBe(false);
  });

  it("does not lead before the 360 has answered", () => {
    expect(offerResearchOnOverview({ ...bare, view: undefined })).toBe(false);
  });
});
