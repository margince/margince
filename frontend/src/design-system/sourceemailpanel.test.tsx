// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { SourceEmailPanel } from "./sourceemailpanel";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(withheld: boolean) {
  const open = vi.fn();
  const body =
    "The complete source paragraph. ".repeat(30) +
    "The final requested action.";
  installFetchStub({
    "GET /activities/source/email-presentation": () =>
      jsonResponse({
        id: "source",
        summary: { subject: "Planning request" },
        body,
        occurred_at: "2026-09-01T09:00:00Z",
        from: [],
        to: [],
        cc: [],
        bcc: [],
        bcc_withheld: false,
        attachments: [],
        links: [],
        access: { content_state: withheld ? "withheld" : "available" },
      }),
  });
  render(
    <StoryProviders>
      <SourceEmailPanel
        activityId="source"
        formatWhen={() => "1 September"}
        onOpen={open}
      />
    </StoryProviders>,
  );
  return { open, body };
}

it("shows the complete authorized source and offers its full conversation", async () => {
  const { open, body } = show(false);
  expect(await screen.findByText(body)).toBeTruthy();
  await userEvent.click(
    screen.getByRole("button", { name: "View full thread" }),
  );
  expect(open).toHaveBeenCalledWith("Planning request");
});

it("does not expose source facts or an opening action when access is withheld", async () => {
  show(true);
  await screen.findByText(/not shared with you/i);
  expect(screen.queryByText("Planning request")).toBeNull();
  expect(screen.queryByRole("button", { name: "View full thread" })).toBeNull();
});
