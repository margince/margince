/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { UnsavedGuard, useHasUnsavedChanges } from "../app/unsaved";
import { LocaleProvider } from "../i18n";
import { VCardImportAction, VCardImportPage } from "./vcard-import";

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

async function upload(user: ReturnType<typeof userEvent.setup>) {
  const input = screen
    .getByTestId("vcard-import-file")
    .querySelector("input[type=file]");
  if (!(input instanceof HTMLInputElement)) {
    throw new Error("the page rendered no file input");
  }
  await user.upload(input, vcardFile());
}

function UnsavedProbe() {
  return (
    <output data-testid="unsaved">
      {useHasUnsavedChanges() ? "dirty" : "clean"}
    </output>
  );
}

describe("VCardImport", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    window.location.hash = "";
  });

  // A link, so it opens in a new tab and folds into the narrow header menu as one.
  it("opens the import as a page of its own under contacts", () => {
    render(<VCardImportAction />);
    expect(screen.getByRole("link", { name: "Import vCards" })).toHaveAttribute(
      "href",
      "#/contacts/import",
    );
  });

  it("leads back to the contact list", () => {
    render(<VCardImportPage />);
    const title = screen.getByRole("heading", { level: 1 });
    expect(title).toHaveTextContent("Import vCards");
    // The link that opened the page is gone, so the title holds focus.
    expect(title).toHaveFocus();
    expect(
      screen.getByRole("link", { name: "Back to Contacts" }),
    ).toHaveAttribute("href", "#/contacts");
  });

  // Leaving does not stop the write, so the guard holds exactly the window in
  // which leaving would lose the report of it.
  it("guards the page while an import is in flight", async () => {
    const user = userEvent.setup();
    let answer: (response: Response) => void = () => {};
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            answer = resolve;
          }),
      ),
    );
    render(
      <UnsavedGuard address="#/contacts/import" onKeep={() => {}}>
        {() => (
          <>
            <VCardImportPage />
            <UnsavedProbe />
          </>
        )}
      </UnsavedGuard>,
    );
    expect(screen.getByTestId("unsaved")).toHaveTextContent("clean");

    await upload(user);
    expect(await screen.findByText("Reading cards…")).toBeVisible();
    expect(screen.getByTestId("unsaved")).toHaveTextContent("dirty");

    answer(jsonResponse({ results: [] }));
    expect(
      await screen.findByText("The file contains no cards."),
    ).toBeVisible();
    expect(screen.getByTestId("unsaved")).toHaveTextContent("clean");
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
              contact_id: "01a04fdf-7a3c-75f6-bdf6-5f868ea3a705",
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

    render(<VCardImportPage />);
    await upload(user);

    expect(
      await screen.findByTestId("vcard-import-report"),
    ).toBeInTheDocument();
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
    expect(screen.getByText("Grace Hopper")).toBeInTheDocument();
    // The card written nowhere is the one a reader must act on, so it says so
    // rather than sitting silently under a success message.
    expect(screen.getByText("Possible duplicate")).toBeInTheDocument();
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

    render(<VCardImportPage />);
    await upload(user);

    expect(
      await screen.findByText("The file contains no cards."),
    ).toBeVisible();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe("/v1/contacts/vcard-import");
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

    render(<VCardImportPage />);
    await upload(user);

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

    render(<VCardImportPage />);
    await upload(user);

    expect(await screen.findByText("quarantined")).toBeInTheDocument();
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
  });
});
