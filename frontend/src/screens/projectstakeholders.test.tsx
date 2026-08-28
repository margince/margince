/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { LocaleProvider } from "../i18n";
import { StakeholdersCard } from "./projectsections";

// A project's stakeholders were readable and nothing more: the card listed a
// name and a role, and the only way to seat or unseat anybody was the endpoint
// itself. These specs hold the two verbs, and — the half that is easy to lose —
// hold them ABSENT on a card whose seats the reader's grant withheld.

const PROJECT = "01a02e25-a5ac-7099-8099-581cbf001a99";
const MAI = "01a02be9-2293-75d2-9dd2-3027d9b63dc2";

type Seats = {
  relationship_id: string;
  person_id: string;
  person_name: string | null;
  role: string | null;
}[];

function view(seats: Seats, omitted: string[] = []) {
  return {
    stakeholders: { data: seats, page: { next_cursor: null, has_more: false } },
    sections_omitted: omitted,
  } as unknown as Parameters<typeof StakeholdersCard>[0]["view"];
}

function withheldView() {
  return {
    sections_omitted: ["stakeholders"],
  } as unknown as Parameters<typeof StakeholdersCard>[0]["view"];
}

const mai: Seats = [
  {
    relationship_id: "rel-1",
    person_id: MAI,
    person_name: "Mai Trần",
    role: "sponsor",
  },
];

function renderCard(
  over: Partial<Parameters<typeof StakeholdersCard>[0]> = {},
) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider>
        <StakeholdersCard view={view(mai)} projectId={PROJECT} {...over} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// Every request the card makes, with the people search answering one name so a
// pick is reachable without a real transport.
function stubFetch(
  onWrite: (call: { method: string; url: string; body: unknown }) => void,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { method, url } = request;
      if (method !== "GET") {
        onWrite({
          method,
          url,
          body: request.body ? JSON.parse(await request.text()) : null,
        });
        return new Response(null, { status: 204 });
      }
      if (url.includes("/people")) {
        return new Response(
          JSON.stringify({
            data: [{ id: MAI, full_name: "Mai Trần" }],
            page: { next_cursor: null, has_more: false },
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      return new Response(
        JSON.stringify({ data: [], page: { next_cursor: null } }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    }),
  );
}

// The picker debounces 250ms. Fake timers are armed BEFORE anything types, and
// userEvent is told to advance them, because switching them on afterwards leaves
// the already-scheduled timeout on the real clock: the test then waits out 250ms
// of wall time on a queue it shares with every other jsdom file, which is the
// flake family the frontend rulebook names rather than a wait at all.
const SEARCH_DEBOUNCE_MS = 250;

function setup() {
  return userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
}

function settleSearch() {
  act(() => {
    vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS);
  });
}

// Nothing auto-unmounts here (no global cleanup in the vitest setup), and a
// second card in the document turns every getByTestId into "found multiple".
beforeEach(() => {
  // shouldAdvanceTime, so the fake clock still ticks on its own: react-query's
  // own scheduling and testing-library's waiters run on timers too, and a clock
  // frozen until this file advances it hangs them rather than speeding them up.
  vi.useFakeTimers({ shouldAdvanceTime: true, advanceTimeDelta: 5 });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("the stakeholders on a project", () => {
  it("seats a person in the role the reader picked", async () => {
    const user = setup();
    const writes: { method: string; url: string; body: unknown }[] = [];
    stubFetch((call) => writes.push(call));
    renderCard();

    await user.click(screen.getByTestId("add-project-stakeholder"));
    // Scoped to the dialog: the card's own row carries a button with this
    // person's name too, and picking the row navigates away instead.
    const dialog = within(screen.getByRole("dialog"));
    await user.type(
      dialog.getByPlaceholderText("Search people by name"),
      "mai",
    );
    settleSearch();
    await user.click(await dialog.findByRole("button", { name: "Mai Trần" }));
    await user.click(dialog.getByLabelText("Role"));
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: "Delivery lead",
      }),
    );
    await user.click(dialog.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(writes.length).toBe(1));
    expect(writes[0].method).toBe("PUT");
    expect(writes[0].url).toContain(`/projects/${PROJECT}/stakeholders`);
    expect(writes[0].body).toEqual({
      person_id: MAI,
      role: "delivery_lead",
    });
  });

  it("takes a person off the project by their own id, behind a confirm", async () => {
    const user = setup();
    const writes: { method: string; url: string; body: unknown }[] = [];
    stubFetch((call) => writes.push(call));
    renderCard();

    await user.click(screen.getByTestId("remove-project-stakeholder"));
    // Two steps on purpose: the endpoint archives the edge and the card offers
    // no way back, so the first click must not be the write.
    expect(writes.length).toBe(0);
    await user.click(screen.getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(writes.length).toBe(1));
    expect(writes[0].method).toBe("DELETE");
    expect(writes[0].url).toContain(`/projects/${PROJECT}/stakeholders/${MAI}`);
  });

  // A successful removal unmounts the row the Remove button sits in, so focus
  // restored to the trigger lands on a detached node and drops a keyboard reader
  // on document.body. It goes to the panel's Add control, which survives every
  // removal and is on screen exactly when a Remove button is.
  it("leaves focus on a control that survives the removal", async () => {
    const user = setup();
    stubFetch(() => {});
    renderCard();

    await user.click(screen.getByTestId("remove-project-stakeholder"));
    await user.click(screen.getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(
      screen.getByTestId("add-project-stakeholder"),
    );
  });

  // WITHHELD IS NOT EMPTY. A reader whose grant refused the seats is told so,
  // and offering them a verb over a list they were not allowed to read invites
  // a write against records they cannot see.
  it("offers no verb at all on seats the reader's grant withheld", () => {
    stubFetch(() => {
      throw new Error("a withheld card must not write");
    });
    renderCard({ view: withheldView() });

    expect(screen.queryByTestId("add-project-stakeholder")).toBeNull();
    expect(screen.queryByTestId("remove-project-stakeholder")).toBeNull();
  });

  it("offers no verb on an archived project", () => {
    stubFetch(() => {
      throw new Error("an archived project must not write");
    });
    renderCard({ readOnly: true });

    expect(screen.queryByTestId("add-project-stakeholder")).toBeNull();
    expect(screen.queryByTestId("remove-project-stakeholder")).toBeNull();
  });

  // A refused write stays on screen in the server's own words. A dialog that
  // closed on a 422 would report a seat nobody has.
  it("keeps the dialog open carrying a refused write's reason", async () => {
    const user = setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.method !== "GET") {
          return new Response(
            JSON.stringify({
              type: "about:blank",
              title: "Unprocessable Entity",
              detail: "this person is archived",
            }),
            {
              status: 422,
              headers: { "content-type": "application/problem+json" },
            },
          );
        }
        return new Response(
          JSON.stringify({
            data: [{ id: MAI, full_name: "Mai Trần" }],
            page: { next_cursor: null },
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }),
    );
    renderCard();

    await user.click(screen.getByTestId("remove-project-stakeholder"));
    await user.click(screen.getByRole("button", { name: "Remove" }));

    expect(await screen.findByText(/this person is archived/)).toBeTruthy();
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  it("keeps the seating dialog open on a refused write, and cancels clean", async () => {
    const user = setup();
    const writes: { method: string; url: string; body: unknown }[] = [];
    stubFetch((call) => {
      writes.push(call);
      throw new Error("this person is not on any company here");
    });
    renderCard();

    await user.click(screen.getByTestId("add-project-stakeholder"));
    const dialog = within(screen.getByRole("dialog"));
    await user.type(
      dialog.getByPlaceholderText("Search people by name"),
      "mai",
    );
    settleSearch();
    await user.click(await dialog.findByRole("button", { name: "Mai Trần" }));
    await user.click(dialog.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(writes.length).toBe(1));
    expect(screen.getByRole("dialog")).toBeTruthy();
    // Cancel drops the pick as well as the dialog: a re-opened form holding
    // somebody the reader had abandoned is a write waiting to be made by
    // accident.
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await user.click(screen.getByTestId("add-project-stakeholder"));
    expect(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Save" }),
    ).toBeDisabled();
  });

  // The empty card is where a person seats the FIRST stakeholder, so the verb
  // has to survive an empty list — the state that reads most like "nothing to
  // do here".
  it("offers the add verb on a project with nobody on it yet", () => {
    stubFetch(() => {
      throw new Error("rendering must not write");
    });
    renderCard({ view: view([]) });

    expect(screen.getByTestId("add-project-stakeholder")).toBeTruthy();
    expect(screen.queryByTestId("remove-project-stakeholder")).toBeNull();
  });
});
