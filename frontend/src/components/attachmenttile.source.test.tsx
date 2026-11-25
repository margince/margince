/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider, translate } from "../i18n";
import { AttachmentTile } from "./attachmenttile";

type Attachment = components["schemas"]["Attachment"];

function file(source: string): Attachment {
  return {
    id: "a-1",
    entity_type: "company",
    entity_id: "o-1",
    filename: "sow.pdf",
    category: "contract",
    source,
    captured_by: "human:u-1",
    created_at: "2026-08-01T09:00:00Z",
  };
}

afterEach(cleanup);

describe("the source a file arrived by, on a library tile", () => {
  it("names an upload in the reader's language, never the stored word", () => {
    render(
      <LocaleProvider initial="de">
        <AttachmentTile file={file("upload")} withSource />
      </LocaleProvider>,
    );
    expect(
      screen.getByText(translate("de", "docs.source.upload"), { exact: false }),
    ).toBeInTheDocument();
    expect(screen.queryByText(/\bupload\b/)).toBeNull();
  });

  it("names nothing for a source this build has no label for", () => {
    render(
      <LocaleProvider initial="en">
        <AttachmentTile file={file("imap")} withSource />
      </LocaleProvider>,
    );
    expect(screen.queryByText(/imap/)).toBeNull();
  });
});
