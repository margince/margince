// @vitest-environment jsdom
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it, vi } from "vitest";
import { catalogs } from "../i18n";
import {
  ACCEPTED_CORPUS_ATTR,
  isAcceptedCorpusFile,
  readCorpusFile,
} from "./voice-corpus-file";

const testdata = join(dirname(fileURLToPath(import.meta.url)), "testdata");

function fixture(name: string, type: string): File {
  return new File([readFileSync(join(testdata, name))], name, { type });
}

// The two documents carry the same letter, so one expectation reads both.
const LETTER = [
  "Dear Sam,",
  "Thanks for the call on Tuesday. Here is the proposal",
  "We can start in May",
  "and finish before the summer break.",
  "Best,",
  "Lars",
];

describe("readCorpusFile — what a dropped file is read as", () => {
  it("reads a plain text file as its own text", async () => {
    const text = await readCorpusFile(
      new File(["Plain prose I wrote."], "notes.md", { type: "text/markdown" }),
    );
    expect(text).toBe("Plain prose I wrote.");
  });

  it("extracts the text of a PDF, page by page in reading order", async () => {
    const text = await readCorpusFile(
      fixture("proposal.pdf", "application/pdf"),
    );
    expect(text).not.toBeNull();
    for (const line of LETTER) {
      expect(text).toContain(line);
    }
    // Page order is reading order: the opening lands before the sign-off.
    expect(text?.indexOf("Dear Sam")).toBeLessThan(text?.indexOf("Lars") ?? -1);
  });

  it("reads a PDF with no text layer as empty rather than refusing it", async () => {
    // A scan is a readable PDF that holds no words; the intake's own empty
    // gate is what names that for the reader, so the reader must not refuse.
    expect(await readCorpusFile(fixture("scan.pdf", "application/pdf"))).toBe(
      "",
    );
  });

  it("extracts the paragraphs of a Word document with breaks, tabs and entities honoured", async () => {
    const text = await readCorpusFile(
      fixture(
        "proposal.docx",
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
      ),
    );
    expect(text).toBe(
      [
        "Dear Sam,",
        "Thanks for the call on Tuesday. Here is the proposal & the timeline we discussed.",
        "We can start in May\nand finish before the summer break.",
        "Best,\tLars",
      ].join("\n"),
    );
  });

  it("answers null for a file that cannot be opened, naming it in the console", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    try {
      expect(
        await readCorpusFile(
          new File(["not a pdf at all"], "broken.pdf", {
            type: "application/pdf",
          }),
        ),
      ).toBeNull();
      expect(
        await readCorpusFile(
          new File(["not a zip either"], "broken.docx", {
            type: "application/octet-stream",
          }),
        ),
      ).toBeNull();
      for (const name of ["broken.pdf", "broken.docx"]) {
        expect(warn).toHaveBeenCalledWith(
          expect.stringContaining("could not be read"),
          name,
          expect.anything(),
        );
      }
    } finally {
      warn.mockRestore();
    }
  });

  it("answers null for a name it has no reader for, without reading it", async () => {
    expect(
      await readCorpusFile(
        new File(["binary"], "photo.png", { type: "image/png" }),
      ),
    ).toBeNull();
  });
});

describe("which files are offered to the server at all", () => {
  it("accepts every format that has a reader, whatever the case of the extension", () => {
    for (const name of [
      "a.txt",
      "b.md",
      "c.vtt",
      "d.srt",
      "e.json",
      "f.pdf",
      "g.docx",
      "H.PDF",
      "letter.final.DOCX",
    ]) {
      expect(isAcceptedCorpusFile(name)).toBe(true);
    }
  });

  it("rejects everything else by name, before any upload", () => {
    for (const name of [
      "photo.png",
      "sheet.xlsx",
      "old.doc",
      "noext",
      "pdf",
      "ends-with-dot.",
    ]) {
      expect(isAcceptedCorpusFile(name)).toBe(false);
    }
  });

  it("offers the picker exactly the formats it can read", () => {
    const offered = ACCEPTED_CORPUS_ATTR.split(",");
    expect(offered).toContain(".pdf");
    expect(offered).toContain(".docx");
    for (const extension of offered) {
      expect(isAcceptedCorpusFile(`sample${extension}`)).toBe(true);
    }
  });
});

// The formats are spelled to the reader in copy, in every locale and on both
// surfaces, and nothing else ties those sentences to the reader table. This
// holds them together: a format added above without a mention here, or a
// mention of one the table cannot read, fails in both directions.
describe("the copy that names the accepted formats", () => {
  const formatLines = [
    "ob.conv.voice.dropHint",
    "ob.conv.voice.fileSkipped",
    "settings.voice.dropHint",
    "settings.voice.noticeSkippedType",
  ] as const;
  const accepted = ACCEPTED_CORPUS_ATTR.split(",");

  it.each(Object.entries(catalogs))(
    "names every accepted format, and no other, in %s",
    (_locale, catalog) => {
      for (const key of formatLines) {
        const line = catalog[key];
        for (const extension of accepted) {
          expect(line, `${key} lacks ${extension}`).toContain(extension);
        }
        const named = line.match(/\.[a-z0-9]+\b/g) ?? [];
        for (const extension of named) {
          expect(accepted, `${key} names ${extension}`).toContain(extension);
        }
      }
    },
  );
});
