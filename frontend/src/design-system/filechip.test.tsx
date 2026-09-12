/** @vitest-environment happy-dom */
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FileChip, previewMediaType } from "./filechip";

// The card IS the download, and its name is the filename — the name the saved
// copy carries, and the only thing that tells two files on one row apart.

describe("FileChip", () => {
  it("downloads under the filename it is named by", () => {
    render(<FileChip href="/v1/attachments/a-1" filename="GR-2026-0092.pdf" />);
    const link = screen.getByRole("link", { name: "GR-2026-0092.pdf" });
    expect(link.getAttribute("href")).toBe("/v1/attachments/a-1");
    expect(link.getAttribute("download")).toBe("GR-2026-0092.pdf");
  });

  it("draws a PDF, an image and anything else with three different marks", () => {
    // The glyph is decorative, so it is compared as markup rather than looked
    // up by name: what matters is that the three kinds do not draw the same
    // one, and that the extension is read case-insensitively — a scanner
    // writes .PDF as readily as .pdf, a phone camera .JPG as readily as .jpg.
    const glyph = (filename: string) =>
      render(
        <FileChip href="/v1/attachments/a" filename={filename} />,
      ).container.querySelector("svg")?.innerHTML;
    const other = glyph("terms-redline.docx");
    const asPdf = glyph("signed.PDF");
    const asImage = glyph("~WRD0005.JPG");
    expect(other).toBeTruthy();
    expect(asPdf).toBeTruthy();
    expect(asImage).toBeTruthy();
    expect(new Set([other, asPdf, asImage]).size).toBe(3);
    expect(glyph("photo.png")).toBe(asImage);
  });

  it("offers to draw only what a browser can show, read from the extension", () => {
    // Case-insensitively, because a scanner writes .PDF as readily as .pdf.
    expect(previewMediaType("signed.PDF")).toBe("application/pdf");
    expect(previewMediaType("~WRD0005.JPG")).toBe("image/jpeg");
    // A picture to a reader, and to no browser: it keeps the picture mark on
    // the card and stays a download.
    expect(previewMediaType("from-the-phone.heic")).toBeNull();
    expect(previewMediaType("terms-redline.docx")).toBeNull();
    expect(previewMediaType("README")).toBeNull();
  });

  it("never offers to draw a kind that can carry script", () => {
    // The preview draws from a blob under this origin, so a document the
    // browser would execute is one running as our own page with our own
    // cookies. SVG is the one that looks like a picture and is not.
    for (const carrier of ["logo.svg", "notes.html", "page.htm", "feed.xml"]) {
      expect(previewMediaType(carrier)).toBeNull();
    }
  });

  it("stamps the kind on the card without adding it to the name", () => {
    const { container } = render(
      <FileChip href="/v1/attachments/a-1" filename="scan.jpg" />,
    );
    const card = within(container);
    expect(card.getByRole("link", { name: "scan.jpg" })).toBeTruthy();
    expect(card.getByText("JPG").getAttribute("aria-hidden")).toBe("true");
  });
});
