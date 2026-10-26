/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { LogActivityAction } from "./logactivity.action";

afterEach(cleanup);

function openDrawer() {
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }, { seat: "full" }),
    "POST /activities": () =>
      jsonResponse({ id: "a-new", kind: "note", subject: "Recap" }, 201),
  });
  render(
    <StoryProviders>
      <LogActivityAction entityType="deal" entityId="d1" openOnMount />
    </StoryProviders>,
  );
}

describe("closing the drawer over a typed note", () => {
  it("asks first, and keeps the drawer open", async () => {
    const user = userEvent.setup();
    openDrawer();
    await user.type(screen.getByLabelText("Subject *"), "Recap");
    await user.keyboard("{Escape}");
    expect(
      screen.getByRole("dialog", { name: "Discard unsaved changes?" }),
    ).toBeTruthy();
    expect(screen.getByRole("dialog", { name: "Log activity" })).toBeTruthy();
  });

  it("closes without asking once the note is saved", async () => {
    const user = userEvent.setup();
    openDrawer();
    await user.type(screen.getByLabelText("Subject *"), "Recap");
    await user.click(screen.getByRole("button", { name: "Log" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
});
