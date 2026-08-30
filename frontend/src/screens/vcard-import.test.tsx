/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { VCardImport } from "./vcard-import";

// The report is the point. An import that says "done" while three cards went
// nowhere is worse than one that refuses, because nobody can tell WHO is
// missing — so every card in the file has to appear, including the ones
// nothing was written for.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function vcardFile() {
  return new File(
    ["BEGIN:VCARD\nVERSION:3.0\nFN:Ada Lovelace\nEND:VCARD"],
    "cards.vcf",
    { type: "text/vcard" },
  );
}

async function openAndUpload(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByTestId("vcard-import"));
  const input = screen
    .getByTestId("vcard-import-file")
    .querySelector("input[type=file]");
  if (!(input instanceof HTMLInputElement)) {
    throw new Error("the dialog rendered no file input");
  }
  await user.upload(input, vcardFile());
}

describe("VCardImport", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("names every card and what became of it", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          results: [
            { index: 0, full_name: "Ada Lovelace", outcome: "created" },
            { index: 1, full_name: "Grace Hopper", outcome: "updated" },
            {
              index: 2,
              full_name: "Alan Turing",
              outcome: "needs_review",
              person_id: "01a04fdf-7a3c-75f6-bdf6-5f868ea3a705",
            },
            {
              index: 3,
              full_name: "Unnamed",
              outcome: "skipped",
              reason: "the card carried no usable name",
            },
          ],
        }),
      ),
    );

    render(<VCardImport />);
    await openAndUpload(user);

    expect(
      await screen.findByTestId("vcard-import-report"),
    ).toBeInTheDocument();
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
    expect(screen.getByText("Grace Hopper")).toBeInTheDocument();
    // The card written nowhere is the one a reader must act on, so it says so
    // rather than sitting silently under a success message.
    expect(
      screen.getByText("Looks like someone you already have"),
    ).toBeInTheDocument();
    // A skipped card names its reason, or nobody can tell who is missing.
    expect(
      screen.getByText("the card carried no usable name"),
    ).toBeInTheDocument();
  });

  it("sends the file as the part the endpoint takes", async () => {
    const user = userEvent.setup();
    const fetchSpy = vi.fn(
      async (_url: string, _init: RequestInit): Promise<Response> =>
        jsonResponse({ results: [] }),
    );
    vi.stubGlobal("fetch", fetchSpy);

    render(<VCardImport />);
    await openAndUpload(user);

    expect(await screen.findByText("That file held no cards.")).toBeVisible();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe("/v1/people/vcard-import");
    expect(init.method).toBe("POST");
    // The endpoint takes a multipart part named `file`; a JSON body or a
    // differently named part reaches a handler that refuses it.
    const body = init.body;
    if (!(body instanceof FormData)) {
      throw new Error("the import sent something other than a multipart body");
    }
    expect(body.get("file")).toBeInstanceOf(File);
  });

  it("shows a refused file as a refusal, not as an empty success", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(
          {
            title: "Unprocessable Entity",
            detail: "card 2 could not be read",
            status: 422,
          },
          422,
        ),
      ),
    );

    render(<VCardImport />);
    await openAndUpload(user);

    expect(await screen.findByTestId("vcard-import-error")).toBeInTheDocument();
    expect(screen.queryByTestId("vcard-import-report")).not.toBeInTheDocument();
  });

  it("survives an outcome this build has no name for", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          results: [
            { index: 0, full_name: "Ada Lovelace", outcome: "quarantined" },
          ],
        }),
      ),
    );

    render(<VCardImport />);
    await openAndUpload(user);

    expect(await screen.findByText("quarantined")).toBeInTheDocument();
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
  });
});
