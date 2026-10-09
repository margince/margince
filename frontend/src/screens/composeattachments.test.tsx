/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ACCEPTED_ATTACHMENT_ATTR } from "./attachmentupload";
import { AttachAction, CarriageNotice } from "./composeattachments";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("CarriageNotice", () => {
  it.each([
    [1, "its 1 attachment cannot be sent there"],
    [3, "its 3 attachments cannot be sent there"],
  ])(
    "counts the attachments on a channel without files (%i)",
    (count, phrase) => {
      render(
        <LocaleProvider initial="en">
          <CarriageNotice channel="SMS" blocks={[{ kind: "carries", count }]} />
        </LocaleProvider>,
      );
      expect(screen.getByRole("listitem")).toHaveTextContent(phrase);
    },
  );
});

describe("AttachAction", () => {
  it("offers only the kinds of file the server will keep", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ data: [] }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
      ),
    );
    render(
      <QueryClientProvider client={new QueryClient()}>
        <LocaleProvider initial="en">
          <AttachAction
            entityType="deal"
            entityId="deal-1"
            chosen={[]}
            onChange={() => {}}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Attach" }));

    const picker = await screen.findByLabelText(/Upload file/);
    expect(picker.getAttribute("accept")).toBe(ACCEPTED_ATTACHMENT_ATTR);
  });
});
