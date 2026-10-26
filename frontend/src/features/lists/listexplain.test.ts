// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import { linkedText } from "./listexplain";

const t = ((key: keyof typeof en) => en[key]) as Parameters<
  typeof linkedText
>[1];

describe("linkedText", () => {
  it("names what a link holds, hides only the entries that are hidden, and says an empty link is empty", () => {
    expect(
      linkedText(
        [
          { value: "t1", label: "Wanted", hidden: false },
          { value: "", hidden: true },
        ],
        t,
      ),
    ).toBe(`Wanted, ${en["lists.why.hidden"]}`);
    expect(linkedText([], t)).toBe(en["lists.why.empty"]);
  });

  it("says more exist when the server cut the reading", () => {
    expect(
      linkedText([{ value: "t1", label: "Wanted", hidden: false }], t, true),
    ).toBe(`Wanted, ${en["lists.why.moreLinked"]}`);
  });
});
