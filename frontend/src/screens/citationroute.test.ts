// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Where a cited record opens.
//
// The account page's commitment rows have passed `source_activity_id` into a
// button since the day they were written, and pressing it did nothing: an
// activity had no detail route, so it fell through the routing branch and the
// receipt branch alike. The email drawer is that route now.

import { describe, expect, it } from "vitest";
import { citationOpensEmail } from "./organizations";

describe("citationOpensEmail", () => {
  it("opens the message for an activity", () => {
    expect(citationOpensEmail("activity")).toBe(true);
  });

  // The kinds that route to a screen or a receipt keep doing so. An email
  // drawer opened over a deal would put the reader on a message that has
  // nothing to do with what they clicked.
  it.each(["deal", "person", "fact", "profile_field", "organization"])(
    "leaves %s to its own destination",
    (kind) => {
      expect(citationOpensEmail(kind)).toBe(false);
    },
  );
});
