/** @vitest-environment jsdom */
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FileChip } from "./filechip";
import { FilePreviewProvider } from "./filepreview";

// A file a reader clicks opens HERE, over the record they are reading, and the
// verbs over it are the ones a reader of a document has. What the cases below
// hold is the part a screenshot cannot: that the bytes are re-typed from the
// filename rather than trusted from the response, that each kind is drawn and
// printed the only way that kind allows, and that a file nothing can draw is
// still the download it always was.

const PDF_BYTES = new Uint8Array([0x25, 0x50, 0x44, 0x46]);

type MintedUrl = Readonly<{ blob: Blob; url: string }>;

let minted: MintedUrl[] = [];
let revoked: string[] = [];

beforeEach(() => {
  minted = [];
  revoked = [];
  // jsdom ships neither, and the preview is built on both: the object URL is
  // what lets a frame draw bytes that arrived as a download.
  Object.assign(URL, {
    createObjectURL: (blob: Blob) => {
      const url = `blob:preview/${minted.length}`;
      minted.push({ blob, url });
      return url;
    },
    revokeObjectURL: (url: string) => {
      revoked.push(url);
    },
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function serving(read: () => Promise<Response>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => read()),
  );
}

function bytes() {
  return Promise.resolve(new Response(PDF_BYTES, { status: 200 }));
}

function Files({ filename }: Readonly<{ filename: string }>) {
  return (
    <FilePreviewProvider>
      <FileChip href="/v1/attachments/a-1" filename={filename} />
    </FilePreviewProvider>
  );
}

async function openPreview(filename: string) {
  const user = userEvent.setup();
  render(<Files filename={filename} />);
  await user.click(screen.getByRole("link", { name: filename }));
  return { user, dialog: await screen.findByRole("dialog") };
}

describe("FilePreview", () => {
  it("opens the file over the page rather than sending it away", async () => {
    serving(bytes);
    const { dialog } = await openPreview("GR-2026-0092.pdf");
    expect(
      within(dialog).getByRole("heading", { name: "GR-2026-0092.pdf" }),
    ).toBeTruthy();
    // The frame arrives with the bytes, so the reader is told the file is
    // opening rather than shown an empty stage.
    const frame = await waitFor(() => {
      const found = dialog.querySelector("iframe");
      expect(found).toBeTruthy();
      return found;
    });
    expect(frame?.getAttribute("src")).toBe("blob:preview/0");
    expect(frame?.getAttribute("title")).toBe("GR-2026-0092.pdf");
  });

  it("types the bytes from the filename, never from the response", async () => {
    // The response calls itself HTML, which is what a store holding a file
    // somebody uploaded may well say. A blob URL inherits this origin, so a
    // frame drawing that as a document would be running it as our own page —
    // the type therefore comes from the extension and from the allowlist.
    serving(() =>
      Promise.resolve(
        new Response(PDF_BYTES, {
          status: 200,
          headers: { "Content-Type": "text/html" },
        }),
      ),
    );
    await openPreview("GR-2026-0092.pdf");
    await waitFor(() => expect(minted).toHaveLength(1));
    expect(minted[0]?.blob.type).toBe("application/pdf");
  });

  it("leaves a file it cannot draw as the download it was", async () => {
    serving(bytes);
    const user = userEvent.setup();
    render(<Files filename="terms-redline.docx" />);
    // What the card did with the press, read where the browser would read it.
    // The listener also stops jsdom from following the link it did not cancel
    // — a navigation it cannot perform, and twelve lines of stack in the middle
    // of a passing run teach a reader to skim the output.
    let cancelled: boolean | null = null;
    const watch = (event: MouseEvent) => {
      cancelled = event.defaultPrevented;
      event.preventDefault();
    };
    document.addEventListener("click", watch);
    await user.click(screen.getByRole("link", { name: "terms-redline.docx" }));
    document.removeEventListener("click", watch);
    expect(cancelled).toBe(false);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("asks the frame to print itself, once there is a document in it", async () => {
    let opened: (response: Response) => void = () => {};
    serving(
      () =>
        new Promise<Response>((resolve) => {
          opened = resolve;
        }),
    );
    const { user, dialog } = await openPreview("GR-2026-0092.pdf");
    // No verb while the stage is still empty: a control for a document that
    // has not arrived is one whose only possible answer is a refusal.
    expect(within(dialog).queryByRole("button", { name: "Print" })).toBeNull();

    opened(new Response(PDF_BYTES, { status: 200 }));
    const frame = await waitFor(() => {
      const found = dialog.querySelector("iframe");
      expect(found).toBeTruthy();
      return found as HTMLIFrameElement;
    });
    const printed = vi.fn();
    Object.defineProperty(frame.contentWindow, "print", { value: printed });

    await user.click(within(dialog).getByRole("button", { name: "Print" }));
    expect(printed).toHaveBeenCalledOnce();
  });

  it("fits a picture to the stage rather than drawing it at its own size", async () => {
    // Not the frame the PDF gets: a browser fits an image to the window only
    // when the image IS the document, so a scan inside a frame arrives at 1:1
    // and a reader meets an A4 page two inches from their face.
    serving(bytes);
    const { user, dialog } = await openPreview("site-photo.jpg");
    const shown = await waitFor(() => {
      const found = dialog.querySelector("img");
      expect(found).toBeTruthy();
      return found as HTMLImageElement;
    });
    expect(shown.getAttribute("src")).toBe("blob:preview/0");
    expect(shown.getAttribute("alt")).toBe("site-photo.jpg");
    expect(dialog.querySelector("iframe")).toBeNull();

    // Drawn by this page, so the PAGE is what prints; the `@media print` rules
    // are what leave the picture alone on the sheet.
    const printed = vi.fn();
    vi.stubGlobal("print", printed);
    await user.click(within(dialog).getByRole("button", { name: "Print" }));
    expect(printed).toHaveBeenCalledOnce();
  });

  it("saves under the filename, from the href the card carries", async () => {
    serving(bytes);
    const { user, dialog } = await openPreview("GR-2026-0092.pdf");
    const clicked: HTMLAnchorElement[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(
      function saved(this: HTMLAnchorElement) {
        clicked.push(this);
      },
    );
    await user.click(within(dialog).getByRole("button", { name: "Download" }));
    expect(clicked).toHaveLength(1);
    expect(clicked[0]?.getAttribute("download")).toBe("GR-2026-0092.pdf");
    expect(clicked[0]?.getAttribute("href")).toBe("/v1/attachments/a-1");
    // The link the save rode on is not left in the document behind it.
    expect(document.querySelectorAll("a[download]")).toHaveLength(1);
    vi.restoreAllMocks();
  });

  it("says a file could not be shown without saying why", async () => {
    serving(() => Promise.resolve(new Response("no", { status: 403 })));
    const { dialog } = await openPreview("GR-2026-0092.pdf");
    expect(
      await within(dialog).findByText("This file cannot be shown here"),
    ).toBeTruthy();
    expect(dialog.textContent).not.toContain("403");
    // The one move left is still in the band above the sentence, and Print is
    // not: there is no document on the stage for it to put on paper.
    expect(
      within(dialog).getByRole("button", { name: "Download" }),
    ).toBeTruthy();
    expect(within(dialog).queryByRole("button", { name: "Print" })).toBeNull();
  });

  it("puts the file away and releases the bytes it was holding", async () => {
    serving(bytes);
    const { user, dialog } = await openPreview("GR-2026-0092.pdf");
    await waitFor(() => expect(minted).toHaveLength(1));
    await user.click(
      within(dialog).getByRole("button", { name: "Close preview" }),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(revoked).toEqual(["blob:preview/0"]);
  });
});
