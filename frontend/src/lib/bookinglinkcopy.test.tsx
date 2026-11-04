/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { stubClipboard } from "../design-system/clipboard-testing";
import { LocaleProvider } from "../i18n";
import { useCopyBookingLink } from "./bookinglinkcopy";

const LABELS = {
  copy: "Copy link",
  copied: "Copied",
  remedy: "Copy failed. Select the link and copy it manually.",
} as const;

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function CopyLink() {
  const copy = useCopyBookingLink("https://crm.example.test/book/alex", LABELS);
  return (
    <>
      <button type="button" onClick={copy.copy}>
        {copy.label}
      </button>
      {copy.notice}
    </>
  );
}

function renderCopy() {
  // No wait between attempts, so a retried mark lands inside the test.
  const client = new QueryClient({
    defaultOptions: { mutations: { retryDelay: 0 } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CopyLink />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function stubShare() {
  return vi
    .spyOn(api, "POST")
    .mockResolvedValue({ data: undefined, error: undefined } as never);
}

describe("useCopyBookingLink", () => {
  it("tells the server the page was shared once the link is copied", async () => {
    const user = userEvent.setup();
    const clipboard = stubClipboard("accepts");
    const share = stubShare();
    renderCopy();

    await user.click(screen.getByRole("button", { name: "Copy link" }));

    await waitFor(() => expect(share).toHaveBeenCalledTimes(1));
    expect(share).toHaveBeenCalledWith("/scheduling/profile/shared");
    expect(clipboard.written).toEqual(["https://crm.example.test/book/alex"]);
  });

  it("tells the server nothing when the copy failed", async () => {
    const user = userEvent.setup();
    stubClipboard("refuses");
    const share = stubShare();
    renderCopy();

    await user.click(screen.getByRole("button", { name: "Copy link" }));

    await screen.findByText(LABELS.remedy);
    expect(share).not.toHaveBeenCalled();
  });

  it("asks again when the mark did not reach the server", async () => {
    const user = userEvent.setup();
    stubClipboard("accepts");
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const share = vi
      .spyOn(api, "POST")
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockResolvedValue({ data: undefined, error: undefined } as never);
    renderCopy();

    await user.click(screen.getByRole("button", { name: "Copy link" }));

    await waitFor(() => expect(share).toHaveBeenCalledTimes(2));
    expect(warn).not.toHaveBeenCalled();
  });

  it("does not ask again once the server refused the mark", async () => {
    const user = userEvent.setup();
    stubClipboard("accepts");
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const share = vi.spyOn(api, "POST").mockResolvedValue({
      data: undefined,
      error: { status: 404, title: "Not found" },
    } as never);
    renderCopy();

    await user.click(screen.getByRole("button", { name: "Copy link" }));

    await waitFor(() => expect(warn).toHaveBeenCalledTimes(1));
    expect(share).toHaveBeenCalledTimes(1);
  });
});
