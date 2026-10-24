/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { type Locale, LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { AvailabilityCard } from "./meeting-availability";

// The hours half of Meetings, as a reader without the booking grant gets it.
// A host who narrows their hours receives fewer bookings and may not connect
// the two, so the card says so at the moment they save.

type WorkingHours = components["schemas"]["WorkingHours"];

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const FALLBACK: WorkingHours = {
  start_time: "09:00",
  end_time: "17:00",
  days: [1, 2, 3, 4, 5],
  timezone: "Europe/Berlin",
};

// backendFor answers the read with the given state and applies a PUT the way
// the server does. It stores what it was sent and answers with it, now chosen.
function backendFor(chosen: boolean, hours: WorkingHours = FALLBACK) {
  let state = { chosen, working_hours: hours };
  const writes: WorkingHours[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({}));
      }
      if (req.url.includes("/me/working-hours")) {
        if (req.method === "PUT") {
          const body = (await req.json()) as WorkingHours;
          writes.push(body);
          state = { chosen: true, working_hours: body };
        }
        return jsonResponse(state);
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, writes: () => writes };
}

const render = (ui: ReactNode, locale: Locale = "en") => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <SavePageProvider>
          {ui}
          <PageSaveBar />
        </SavePageProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("AvailabilityCard, hours only", () => {
  it("shows the effective server timezone before the first save", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(false, { ...FALLBACK, timezone: "Asia/Bangkok" }).fetchMock,
    );
    render(<AvailabilityCard hoursOnly />);
    expect(
      (await screen.findByRole("combobox", { name: "Timezone" })).textContent,
    ).toContain("Bangkok");
  });

  it("offers the fallback as a starting point rather than as somebody's decision", async () => {
    vi.stubGlobal("fetch", backendFor(false).fetchMock);
    render(<AvailabilityCard hoursOnly />);

    // The sentence is the whole difference between "nobody has chosen" and
    // "somebody chose this": a reader who cannot tell them apart does not know
    // whether the hours below are theirs.
    expect(await screen.findByText(/Not set yet/i)).not.toBeNull();
  });

  // A body that lost the reading: contract-required, so malformed, and
  // dereferencing it once took the whole settings page down.
  it("says the reading is unavailable rather than taking the page down", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        return url.includes("/me/working-hours")
          ? jsonResponse({ chosen: false })
          : jsonResponse(meFixture({}));
      }),
    );
    render(<AvailabilityCard hoursOnly />);

    expect(await screen.findByText(/Some data did not load/i)).not.toBeNull();
    expect(screen.queryByRole("button", { name: /save/i })).toBeNull();
  });

  it("says nothing about an unset choice once one has been made", async () => {
    vi.stubGlobal("fetch", backendFor(true).fetchMock);
    render(<AvailabilityCard hoursOnly />);

    await screen.findByLabelText("Saturday");
    expect(screen.queryByText(/Not set yet/i)).toBeNull();
    // A saved card with nothing changed has nothing to commit.
    expect(screen.queryByRole("button", { name: /save/i })).toBeNull();
  });

  it("writes the days ascending, whatever order they were ticked in", async () => {
    const user = userEvent.setup();
    const backend = backendFor(false);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AvailabilityCard hoursOnly />);

    const saturday = await screen.findByLabelText("Saturday");
    await user.click(saturday);
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(backend.writes()).toHaveLength(1));
    // Ascending: the days are a set, and the server stores them that way, so a
    // client that sent them in click order would make the two disagree about
    // what one week looks like.
    expect(backend.writes()[0].days).toEqual([1, 2, 3, 4, 5, 6]);
  });

  it("says what narrowing the hours will do, at the moment it is saved", async () => {
    const user = userEvent.setup();
    const backend = backendFor(true);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AvailabilityCard hoursOnly />);

    // Monday to Thursday instead of Friday: one day fewer, which is a narrower
    // week however long each day is.
    const friday = await screen.findByLabelText("Friday");
    await user.click(friday);
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText(/Fewer bookable hours/i)).not.toBeNull();
  });

  it("says nothing of the kind when the week did not narrow", async () => {
    const user = userEvent.setup();
    const backend = backendFor(true);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AvailabilityCard hoursOnly />);

    const saturday = await screen.findByLabelText("Saturday");
    await user.click(saturday);
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(backend.writes()).toHaveLength(1));
    // A warning that fires on every save is one a reader learns to skip, and
    // then it is not there on the save it was written for.
    expect(screen.queryByText(/Fewer bookable hours/i)).toBeNull();
  });

  it("chooses a city with spaces by keyboard and saves its timezone identifier", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-01-15T12:00:00Z"));
    const user = userEvent.setup();
    const backend = backendFor(true);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AvailabilityCard hoursOnly />);

    const picker = await screen.findByRole("combobox", { name: "Timezone" });
    expect(picker.textContent).toContain("Berlin (Europe)");
    await user.click(picker);
    await user.keyboard("New York{Enter}");
    expect(picker.textContent).toContain("New York (America) · UTC-5");
    await user.click(picker);
    await user.keyboard("Bangkok{Enter}");
    expect(picker.textContent).toContain("Bangkok (Asia) · UTC+7");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(backend.writes()).toHaveLength(1));
    expect(backend.writes()[0].timezone).toBe("Asia/Bangkok");
  });
});
