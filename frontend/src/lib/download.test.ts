/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import {
  downloadBytes,
  downloadFrom,
  filenameFromDisposition,
} from "./download";

afterEach(() => {
  vi.restoreAllMocks();
  Reflect.deleteProperty(URL, "createObjectURL");
  Reflect.deleteProperty(URL, "revokeObjectURL");
});

describe("filenameFromDisposition", () => {
  it("reads the name the server sent", () => {
    expect(
      filenameFromDisposition(
        'attachment; filename="contact-export.csv"',
        "fallback.csv",
      ),
    ).toBe("contact-export.csv");
  });

  it.each([
    ["no header at all", null],
    ["a header with no filename", "attachment"],
    ["an empty name", 'attachment; filename=""'],
    [
      "a broken escape in the only name",
      "attachment; filename*=UTF-8''%E0%A4%A",
    ],
    [
      "another charset in the only name",
      "attachment; filename*=ISO-8859-1''r%E9sum%E9.csv",
    ],
    ["a name that is only a separator", 'attachment; filename="/"'],
    ["a name that is only dots", 'attachment; filename=".."'],
  ])("falls back on %s", (_name, header) => {
    expect(filenameFromDisposition(header, "fallback.csv")).toBe(
      "fallback.csv",
    );
  });

  it("unescapes a quote in the quoted name the way the server writes it", () => {
    expect(
      filenameFromDisposition(
        'attachment; filename="a\\"b.txt"',
        "fallback.csv",
      ),
    ).toBe('a"b.txt');
  });

  it("prefers the extended name over its ASCII fallback", () => {
    expect(
      filenameFromDisposition(
        "attachment; filename=\"Angebot M_ller.txt\"; filename*=UTF-8''Angebot%20M%C3%BCller.txt",
        "fallback.csv",
      ),
    ).toBe("Angebot Müller.txt");
  });

  it("uses the ASCII fallback when the extended name is broken", () => {
    expect(
      filenameFromDisposition(
        "attachment; filename=\"M_ller.txt\"; filename*=UTF-8''%E0%A4%A",
        "fallback.csv",
      ),
    ).toBe("M_ller.txt");
  });

  it("keeps only the leaf of a name carrying a path", () => {
    // The header comes from our own server, but it is still input, and a path
    // separator in a download name is how one talks a browser into writing
    // somewhere other than the downloads folder.
    expect(
      filenameFromDisposition(
        'attachment; filename="../../etc/passwd"',
        "fallback.csv",
      ),
    ).toBe("passwd");
  });
});

describe("downloadBytes", () => {
  it("names the file, clicks it, and releases the blob", () => {
    const createObjectURL = vi.fn(() => "blob:test");
    const revokeObjectURL = vi.fn();
    Object.defineProperties(URL, {
      createObjectURL: { configurable: true, value: createObjectURL },
      revokeObjectURL: { configurable: true, value: revokeObjectURL },
    });
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => undefined);

    downloadBytes("id,name\n1,Ann\n", "contact-export.csv", "text/csv");

    expect(createObjectURL).toHaveBeenCalledOnce();
    expect(click).toHaveBeenCalledOnce();
    // The revoke is the step a second copy of this forgets: an object URL pins
    // its blob for the document's lifetime, so a screen that exports repeatedly
    // without it keeps every export it has ever made.
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:test");
  });

  it("has the anchor in the document at the moment it is clicked", () => {
    // Safari ignores `download` on a detached node and saves NOTHING, with no
    // error — so this passes everywhere the suite runs while the export it is
    // about is silently dead in one browser. Asserted at click time rather than
    // after: the anchor is removed again immediately.
    Object.defineProperties(URL, {
      createObjectURL: { configurable: true, value: () => "blob:test" },
      revokeObjectURL: { configurable: true, value: () => undefined },
    });
    let connectedAtClick: boolean | null = null;
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(
      function connected(this: HTMLAnchorElement) {
        connectedAtClick = this.isConnected;
      },
    );

    downloadBytes("id\n1\n", "contact-export.csv", "text/csv");

    expect(connectedAtClick).toBe(true);
  });
});

describe("downloadFrom", () => {
  it("saves an address the page already holds, and releases nothing", () => {
    // The href belongs to whoever made it — a preview still drawing the blob it
    // just saved must not have it revoked out from under the image.
    const revokeObjectURL = vi.fn();
    Object.defineProperties(URL, {
      revokeObjectURL: { configurable: true, value: revokeObjectURL },
    });
    let connectedAtClick: boolean | null = null;
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(
      function connected(this: HTMLAnchorElement) {
        connectedAtClick = this.isConnected;
      },
    );

    downloadFrom("blob:owned-elsewhere", "scan.pdf");

    expect(connectedAtClick).toBe(true);
    expect(revokeObjectURL).not.toHaveBeenCalled();
  });
});
