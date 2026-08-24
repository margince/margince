/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FALLBACK_RECORD_ZONE } from "../format/timezone";
import {
  RecordZoneProvider,
  renderableRecordZone,
  useRecordZone,
} from "./recordzone";

// What the record zone must do with what the server sends it, and what a
// screen under the provider sees.
//
// The stakes are not cosmetic. This zone reaches every date on every record
// page, so the two failures worth proving are the ones that would be silent:
// a zone the browser cannot render taking the application down instead of
// falling back, and a fallback that quietly replaces a zone the installation
// really did configure.

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function ZoneProbe() {
  return <p data-testid="zone">{useRecordZone()}</p>;
}

describe("the zone a record's dates are read in", () => {
  it("serves what the provider was given", () => {
    render(
      <RecordZoneProvider zone="Asia/Ho_Chi_Minh">
        <ZoneProbe />
      </RecordZoneProvider>,
    );
    expect(screen.getByTestId("zone").textContent).toBe("Asia/Ho_Chi_Minh");
  });

  it("falls back for a screen mounted with no provider", () => {
    // A story or a bare test mount. It gets a renderable zone rather than an
    // empty string, which `Intl` rejects — so the surface draws instead of
    // throwing, and what it draws is legibly the fallback.
    render(<ZoneProbe />);
    expect(screen.getByTestId("zone").textContent).toBe(FALLBACK_RECORD_ZONE);
  });
});

describe("resolving the installation's configured zone", () => {
  it("takes a configured zone the browser can render", () => {
    expect(renderableRecordZone("Asia/Ho_Chi_Minh")).toBe("Asia/Ho_Chi_Minh");
  });

  it("falls back while the settings read is still in flight", () => {
    // undefined is "not yet", not "not set". The authenticated boundary holds
    // its paint over this window, so what matters here is only that it resolves
    // to something renderable rather than throwing.
    expect(renderableRecordZone(undefined)).toBe(FALLBACK_RECORD_ZONE);
  });

  it("falls back and REPORTS a zone this browser cannot render", () => {
    // Two zone databases out of step: the server validated the name against
    // its own tzdata, this browser does not know it. Unhandled, the RangeError
    // from Intl takes down every record page for every reader, and the admin
    // who could fix it can no longer load the settings screen either.
    const reported = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);

    expect(renderableRecordZone("Mars/Olympus_Mons")).toBe(
      FALLBACK_RECORD_ZONE,
    );

    // Reported, not swallowed. This state is the server and this browser
    // disagreeing about what a zone name means; silently rendering every date
    // on a different clock than the one the installation chose is the outcome
    // the report exists to prevent going unnoticed.
    expect(reported).toHaveBeenCalled();
    expect(reported.mock.calls.flat().map(String).join(" ")).toContain(
      "Mars/Olympus_Mons",
    );
  });

  it("refuses a fixed offset, which freezes DST rules", () => {
    // The trap `isRenderableZone` exists for: Intl RESOLVES these, so a
    // hand-rolled probe would pass them and then throw inside `formatDate` one
    // line later — in the exact place the fallback exists to protect.
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    for (const offset of ["+01:00", "Etc/GMT-1", "GMT"]) {
      expect(renderableRecordZone(offset)).toBe(FALLBACK_RECORD_ZONE);
    }
  });
});
