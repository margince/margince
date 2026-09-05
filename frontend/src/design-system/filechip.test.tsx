/** @vitest-environment jsdom */
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FileChip } from "./filechip";

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

  it("stamps the kind on the card without adding it to the name", () => {
    const { container } = render(
      <FileChip href="/v1/attachments/a-1" filename="scan.jpg" />,
    );
    const card = within(container);
    expect(card.getByRole("link", { name: "scan.jpg" })).toBeTruthy();
    expect(card.getByText("JPG").getAttribute("aria-hidden")).toBe("true");
  });
});
