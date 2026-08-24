/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { formatDateTime } from "../format/format";
import { FALLBACK_RECORD_ZONE } from "../format/timezone";
import {
  RecordZoneProvider,
  renderableRecordZone,
  useConfiguredRecordZone,
  useRecordZone,
} from "./recordzone";
import { INSTALLATION_SETTINGS_KEY } from "./uploadlimit";

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

describe("an admin changing the zone while a record page is open", () => {
  // The whole feature, end to end, at the seam where it could quietly not work:
  // the settings screen writes the PATCH response straight into the settings
  // cache (`setQueryData`), and nothing tells the record pages. They do not
  // need telling — the date is computed at render time from the zone this
  // context serves, so the write re-renders the provider and every screen under
  // it — but that is a claim about React Query notifying an observer, which is
  // exactly the kind of thing that is true until a library upgrade makes it
  // false. Silently: the dates stay on the old clock and look fine.
  //
  // `waitFor` rather than a bare assertion after the write, because the
  // notification is flushed asynchronously. Asserting immediately reads the DOM
  // one tick early and reports a failure the product does not have.
  it("re-renders an open date on the new clock", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ timezone: "Asia/Ho_Chi_Minh" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
      ),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <ConfiguredRoot />
      </QueryClientProvider>,
    );

    // 18:30 UTC is the 20th in Saigon and the 19th in Berlin — one instant, two
    // calendar days, which is what makes this assertion able to fail.
    await waitFor(() =>
      expect(screen.getByTestId("when").textContent).toContain("20.08.2026"),
    );

    client.setQueryData(INSTALLATION_SETTINGS_KEY, {
      timezone: "Europe/Berlin",
    });

    await waitFor(() =>
      expect(screen.getByTestId("when").textContent).toContain("19.08.2026"),
    );
  });
});

// A record date, rendered the way a screen renders one: through the hook, at
// render time, with no cache of its own.
function WhenProbe() {
  return (
    <p data-testid="when">
      {formatDateTime("2026-08-19T18:30:00Z", "de", useRecordZone())}
    </p>
  );
}

// The authenticated boundary's own wiring, in miniature.
function ConfiguredRoot() {
  const { zone } = useConfiguredRecordZone(true);
  return (
    <RecordZoneProvider zone={zone}>
      <WhenProbe />
    </RecordZoneProvider>
  );
}
