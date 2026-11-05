/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Home opens the row its address names, which is where a priorities mail
// sends the reader.

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { meetingRow, readingsDay } from "../../testing/brief.fixtures";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { BriefScreen } from "./brief";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it("opens the named row beside the day, and closing clears the address", async () => {
  const user = userEvent.setup();
  window.location.hash = "#/home?focus=meeting-m2";
  stubApi({
    "GET /worklist": () =>
      jsonResponse(
        readingsDay({}, [meetingRow("m1", false), meetingRow("m2", true)]),
      ),
  });
  render(<BriefScreen />);

  const dialog = await screen.findByRole("dialog");
  expect(
    within(dialog).getByRole("heading", { name: en["brief.focus.context"] }),
  ).toBeTruthy();

  await user.keyboard("{Escape}");
  await waitFor(() => expect(window.location.hash).not.toContain("focus="));
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("opens nothing for a row the day no longer lists", async () => {
  window.location.hash = "#/home?focus=meeting-gone";
  stubApi({ "GET /worklist": () => jsonResponse(readingsDay()) });
  render(<BriefScreen />);

  expect(
    await screen.findAllByText("Weber GmbH · quarterly review"),
  ).not.toHaveLength(0);
  expect(screen.queryByRole("dialog")).toBeNull();
});
