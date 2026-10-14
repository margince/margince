// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { formatUploadLimit } from "./uploadlimit";

// The number this renders is compared by a reader against their own file, and
// the server refuses on the same one. So it has to be exact in both directions:
// a limit stated too high wastes an upload, one stated too low refuses a file
// the installation would have taken.

describe("stating the upload limit", () => {
  it("renders a whole limit as a whole number", () => {
    expect(formatUploadLimit(25_000_000, "en")).toBe("25 MB");
    expect(formatUploadLimit(3_000_000, "en")).toBe("3 MB");
  });

  it("keeps a decimal rather than rounding a limit away", () => {
    // What a binary constant on the server would produce. Rounded to "8 MB"
    // this would refuse a file at 8.2 MB that the sentence said would fit.
    expect(formatUploadLimit(8 << 20, "en")).toBe("8.4 MB");
    expect(formatUploadLimit(12_500_000, "en")).toBe("12.5 MB");
  });

  it("still describes a limit below a megabyte", () => {
    // Truncating division read this as "0 MB", which tells the reader nothing
    // they can act on.
    expect(formatUploadLimit(900_000, "en")).toBe("0.9 MB");
  });

  it("writes the decimal the way this reader writes decimals", () => {
    // The figure a German reader compares against their own file. Shown with a
    // decimal POINT it reads as twelve thousand five hundred, in a sentence
    // whose other numbers all carry a comma.
    expect(formatUploadLimit(12_500_000, "de")).toBe("12,5 MB");
  });

  it("uses decimal megabytes, the unit the server enforces", () => {
    // 25 MiB is 26,214,400 bytes. Reported as "25 MB" it overstates the real
    // ceiling by 4.8% — the drift this whole unit choice exists to remove.
    expect(formatUploadLimit(25_000_000, "en")).not.toBe(
      formatUploadLimit(25 * 1024 * 1024, "en"),
    );
  });
});
