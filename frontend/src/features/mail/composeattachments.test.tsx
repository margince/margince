/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { ACCEPTED_ATTACHMENT_ATTR } from "../../lib/attachmentupload";
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

  it("takes no upload once the message carries ten files", async () => {
    stubFiles(() => jsonResponse({ data: [] }));
    const ten = Array.from({ length: 10 }, (_, at) => ({
      id: `f-${at}`,
      filename: `file-${at}.pdf`,
    }));
    showAttach(ten, () => {});

    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Attach" }));

    expect(await screen.findByLabelText(/Upload file/)).toBeDisabled();
  });

  it("asks for the file the record already holds, and attaches it once", async () => {
    const sent: FormData[] = [];
    const held = {
      id: "a-1",
      entity_type: "deal",
      entity_id: "deal-1",
      filename: "offer.pdf",
      byte_size: 8,
      source: "upload",
      captured_by: "human:u-1",
      created_at: "2026-06-01T08:00:00Z",
    };
    stubFiles((init) => {
      if (init?.body instanceof FormData) {
        sent.push(init.body);
        return jsonResponse(held);
      }
      return jsonResponse({ data: [] });
    });
    let chosen: readonly { id: string; filename: string }[] = [
      { id: "a-1", filename: "offer.pdf" },
    ];
    showAttach(chosen, (update) => {
      chosen = update(chosen);
    });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Attach" }));

    await user.upload(
      await screen.findByLabelText(/Upload file/),
      new File(["%PDF-1.4"], "offer.pdf", { type: "application/pdf" }),
    );

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]?.get("reuse_identical")).toBe("true");
    await waitFor(() => expect(chosen).toHaveLength(1));
  });
});

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function stubFiles(answer: (init?: RequestInit) => Response) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) =>
      answer(init),
    ),
  );
}

function showAttach(
  chosen: readonly { id: string; filename: string }[],
  onChange: Parameters<typeof AttachAction>[0]["onChange"],
) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <AttachAction
          entityType="deal"
          entityId="deal-1"
          chosen={chosen}
          onChange={onChange}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}
