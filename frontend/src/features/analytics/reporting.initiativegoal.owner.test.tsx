// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { GoalFields, goalOf } from "./reporting.initiativegoal";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;

afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

const roster = {
  data: [
    { id: "u-maya", email: "maya@acme.test", display_name: "Maya Lindqvist" },
    { id: "u-sam", email: "sam@acme.test", display_name: "Sam Okafor" },
  ],
  page: { next_cursor: null, has_more: false },
};

it("names the owner the reader picks on the goal", async () => {
  const user = userEvent.setup();
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /users": () => jsonResponse(roster),
  });
  const onChange = vi.fn();
  const goal = { ...goalOf(), name: "DACH expansion" };
  render(
    <StoryProviders>
      <GoalFields
        goal={goal}
        onChange={onChange}
        onProposal={() => undefined}
      />
    </StoryProviders>,
  );
  await user.click(await screen.findByRole("combobox", { name: /Owner/ }));
  await user.click(
    await within(screen.getByRole("listbox")).findByRole("option", {
      name: /Sam Okafor/,
    }),
  );
  expect(onChange).toHaveBeenLastCalledWith({ ...goal, ownerId: "u-sam" });
});
