/** @vitest-environment jsdom */
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
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { type Locale, LocaleProvider } from "../i18n";
import { WorkingHoursCard } from "./working-hours";

// Settings → Account → when you are bookable. The reader's own setting, so no
// grant fixture appears below: there is no seat that could be refused it, and
// an admin does not set a colleague's week here.
//
// The case that matters most is the LAST one. A host who narrows their hours
// receives fewer bookings and will not necessarily connect the two, so the
// screen says so at the moment they save — the difference between a setting
// and a trap, and the half nothing else in the stack can hold.

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
// the server does — storing what it was sent and answering with it, now chosen.
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
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("WorkingHoursCard", () => {
  it("offers the fallback as a starting point rather than as somebody's decision", async () => {
    vi.stubGlobal("fetch", backendFor(false).fetchMock);
    render(<WorkingHoursCard />);

    // The sentence is the whole difference between "nobody has chosen" and
    // "somebody chose this": a reader who cannot tell them apart does not know
    // whether the hours below are theirs.
    expect(await screen.findByText(/have not chosen yet/i)).not.toBeNull();
  });

  // A body that LOST the reading. The field is contract-required, so this is a
  // malformed answer rather than a state the server offers — and the card sits
  // on the account page, where dereferencing it took the WHOLE PAGE down over
  // one window nobody could edit.
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
    render(<WorkingHoursCard />);

    expect(await screen.findByText(/could not be loaded/i)).not.toBeNull();
    expect(screen.queryByRole("button", { name: /save/i })).toBeNull();
  });

  it("says nothing about an unset choice once one has been made", async () => {
    vi.stubGlobal("fetch", backendFor(true).fetchMock);
    render(<WorkingHoursCard />);

    await screen.findByRole("button", { name: /save/i });
    expect(screen.queryByText(/have not chosen yet/i)).toBeNull();
  });

  it("writes the days ascending, whatever order they were ticked in", async () => {
    const backend = backendFor(false);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<WorkingHoursCard />);

    const saturday = await screen.findByLabelText("Saturday");
    await userEvent.click(saturday);
    await userEvent.click(
      screen.getByRole("button", { name: /save working hours/i }),
    );

    await waitFor(() => expect(backend.writes()).toHaveLength(1));
    // Ascending: the days are a set, and the server stores them that way, so a
    // client that sent them in click order would make the two disagree about
    // what one week looks like.
    expect(backend.writes()[0].days).toEqual([1, 2, 3, 4, 5, 6]);
  });

  it("says what narrowing the hours will do, at the moment it is saved", async () => {
    const backend = backendFor(true);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<WorkingHoursCard />);

    // Monday to Thursday instead of Friday: one day fewer, which is a narrower
    // week however long each day is.
    const friday = await screen.findByLabelText("Friday");
    await userEvent.click(friday);
    await userEvent.click(
      screen.getByRole("button", { name: /save working hours/i }),
    );

    expect(
      await screen.findByText(/bookable for less of the week/i),
    ).not.toBeNull();
  });

  it("says nothing of the kind when the week did not narrow", async () => {
    const backend = backendFor(true);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<WorkingHoursCard />);

    const saturday = await screen.findByLabelText("Saturday");
    await userEvent.click(saturday);
    await userEvent.click(
      screen.getByRole("button", { name: /save working hours/i }),
    );

    await waitFor(() => expect(backend.writes()).toHaveLength(1));
    // A warning that fires on every save is one a reader learns to skip, and
    // then it is not there on the save it was written for.
    expect(screen.queryByText(/bookable for less of the week/i)).toBeNull();
  });
});
