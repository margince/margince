// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

import { composeStories } from "@storybook/react-vite";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import * as stories from "./worklist.row.stories";

afterEach(cleanup);

const { AMeetingWithItsStartTime, AMeetingWithNobodyToBriefAgainst } =
  composeStories(stories);

// The two meeting stories must render DIFFERENTLY, and this is what says so.
//
// They are one absent field apart: `with_person` decides whether the row can
// address the brief at all. A pair of stories that looked alike would let a
// wrong destination ship looking exactly like a right one — which is the whole
// reason the negative story exists, and it is not a claim a story can make
// about itself.
it("draws the brief only for the meeting that names somebody", async () => {
  render(<AMeetingWithItsStartTime />);
  const withSomebody = screen
    .getAllByRole("link")
    .map((el) => el.getAttribute("href"));
  cleanup();

  render(<AMeetingWithNobodyToBriefAgainst />);
  const withNobody = screen
    .queryAllByRole("link")
    .map((el) => el.getAttribute("href"));

  // The address is the assertion, not merely the presence of a link: the row
  // carries other links, and "some link exists" would pass on a row whose
  // brief never rendered.
  expect(withSomebody.some((href) => href?.includes("prep="))).toBe(true);
  expect(withNobody.some((href) => href?.includes("prep="))).toBe(false);
});
