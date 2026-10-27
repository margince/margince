/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { UnsavedGuard, useHasUnsavedChanges } from "../../components/unsaved";
import { LocaleProvider } from "../../i18n";
import { ACCEPTED_ATTACHMENT_EXTENSIONS } from "../../lib/attachmentupload";
import {
  CARD_PHOTO_EXTENSIONS,
  isCardPhoto,
  VCardImportAction,
  VCardImportPage,
} from "./vcard-import";

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
    expect(title).toHaveTextContent("Import business cards");
    // The link that opened the page is gone, so the title holds focus.
    expect(title).toHaveFocus();
    expect(
      screen.getByRole("link", { name: "Back to contacts" }),
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

  it("offers to convert the lead a skipped card names", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          results: [
            {
              index: 0,
              full_name: "Vera Card",
              outcome: "skipped",
              reason:
                "this card names somebody who is already a lead; convert the lead instead",
              lead_id: "lead-7",
            },
          ],
        }),
      ),
    );
    render(<VCardImportPage />);
    await upload(user);
    await user.click(
      await screen.findByRole("button", { name: "Convert the lead" }),
    );
    expect(window.location.hash).toBe("#/leads/lead-7?action=qualify");
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

  it("shows under each card what the import did with every field", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          results: [
            {
              index: 0,
              full_name: "Grace Hopper",
              outcome: "updated",
              fields: [
                { field: "phone", value: "+49301234567", action: "imported" },
                { field: "title", value: "VP Finance", action: "replaced" },
                { field: "address", value: "Hauptstr. 1", action: "kept" },
                { field: "mobile_phone", action: "not_in_card" },
                { field: "website", action: "not_in_card" },
              ],
            },
          ],
        }),
      ),
    );

    render(<VCardImportPage />);
    await upload(user);

    const fields = await screen.findByTestId("vcard-import-fields");
    const row = (value: string) => {
      const item = within(fields).getByText(value).closest("li");
      if (!(item instanceof HTMLElement)) {
        throw new Error(`no field row holds ${value}`);
      }
      return item;
    };
    expect(row("+49301234567")).toHaveTextContent("Phone");
    expect(row("+49301234567")).toHaveTextContent("Imported");
    expect(row("VP Finance")).toHaveTextContent("Title");
    expect(row("VP Finance")).toHaveTextContent(
      "Replaced, old value can be restored",
    );
    expect(row("Hauptstr. 1")).toHaveTextContent("Address");
    expect(row("Hauptstr. 1")).toHaveTextContent("Record value kept");
    // What the card left out is one muted group, named field by field.
    const absent = screen.getByTestId("vcard-import-absent");
    expect(absent).toHaveTextContent("Not in the card");
    expect(within(absent).getByText("Mobile")).toBeInTheDocument();
    expect(within(absent).getByText("Website")).toBeInTheDocument();
  });

  it("empties the page for the next card and puts focus on the file field", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          results: [
            { index: 0, full_name: "Ada Lovelace", outcome: "created" },
          ],
        }),
      ),
    );

    render(<VCardImportPage />);
    await upload(user);
    expect(await screen.findByTestId("vcard-import-report")).toBeVisible();
    expect(screen.getByTestId("vcard-import-file")).toHaveTextContent(
      "cards.vcf",
    );

    await user.click(screen.getByRole("button", { name: "Next vCard" }));

    expect(screen.queryByTestId("vcard-import-report")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Next vCard" })).toBeNull();
    const field = screen.getByTestId("vcard-import-file");
    expect(field).not.toHaveTextContent("cards.vcf");
    expect(field).toHaveTextContent("Select file or photo");
    expect(field.querySelector("input[type=file]")).toHaveFocus();
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

  it("sends a photo to the card photo endpoint and says it waits for approval", async () => {
    const user = userEvent.setup();
    const fetchSpy = vi.fn(
      async (_url: string, _init: RequestInit): Promise<Response> =>
        jsonResponse({
          results: [
            { index: 0, full_name: "Ada Lovelace", outcome: "proposed" },
          ],
        }),
    );
    vi.stubGlobal("fetch", fetchSpy);

    render(<VCardImportPage />);
    const input = screen
      .getByTestId("vcard-import-file")
      .querySelector("input[type=file]");
    if (!(input instanceof HTMLInputElement)) {
      throw new Error("the page rendered no file input");
    }
    await user.upload(
      input,
      new File(["jpeg"], "cards.jpg", { type: "image/jpeg" }),
    );

    expect(await screen.findByText("Waiting for approval")).toBeVisible();
    expect(fetchSpy.mock.calls[0][0]).toBe("/v1/contacts/card-photos");
  });

  it("accepts vCards and photos in the picker", () => {
    render(<VCardImportPage />);
    const input = screen
      .getByTestId("vcard-import-file")
      .querySelector("input[type=file]");
    expect(input).toHaveAttribute("accept", expect.stringContaining(".vcf"));
    expect(input).toHaveAttribute(
      "accept",
      expect.stringContaining("image/jpeg"),
    );
    expect(input).toHaveAttribute(
      "accept",
      expect.stringContaining("image/png"),
    );
    expect(input).not.toHaveAttribute(
      "accept",
      expect.stringMatching(/heic|heif/i),
    );
  });
});

describe("isCardPhoto", () => {
  it("tells a photo from a vCard by type or by extension", () => {
    expect(isCardPhoto(new File([""], "a.vcf", { type: "text/vcard" }))).toBe(
      false,
    );
    expect(isCardPhoto(new File([""], "a.jpg", { type: "image/jpeg" }))).toBe(
      true,
    );
    // A HEIC dragged in still goes to the photo endpoint, which says to save it
    // as JPEG or PNG. It often carries no content type at all.
    expect(isCardPhoto(new File([""], "IMG_1.HEIC", { type: "" }))).toBe(true);
  });

  it("only offers photo types the attachment mirror accepts", () => {
    for (const extension of CARD_PHOTO_EXTENSIONS) {
      expect(ACCEPTED_ATTACHMENT_EXTENSIONS).toContain(extension);
    }
  });
});
