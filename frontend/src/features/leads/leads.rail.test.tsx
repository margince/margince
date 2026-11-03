/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { changeChoice } from "../../testing/choicedialog.testkit";
import { meFixture } from "../../testing/mefixture";
import { RecordShell } from "../../testing/recordshell.testkit";
import { LeadScreen } from "./leads";

// The lead page writes from both of its columns. A write from one control must
// leave another control's draft alone, and a refusal must reach the reader on
// whichever tab they are, said once.
//
// Kept out of `leads.test.tsx` because that file is already well past the
// thousand-line ceiling this tree splits test files at.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

const lead = {
  // The caller's own lead: absent means NOT writable per the contract.
  writable: true,
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted" as const,
  score: 72,
  owner_id: "u-other",
  source: "manual",
  captured_by: "human:u-9",
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * The reads this page makes, answered plainly, with the caller deciding what
 * the PATCH does. Everything unnamed falls through to the lead itself, which
 * is what the screen's other reads (context, signals, history) can take.
 */
function stubFetch(onPatch: (body: unknown) => Response) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      if (pathname.endsWith("/v1/me")) {
        return jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
          authorization: meFixture({ allow: { lead: ["read", "update"] } })
            .authorization,
        });
      }
      if (pathname.endsWith("/v1/users")) {
        return jsonResponse({
          data: [
            { id: "u-9", display_name: "Me", kind: "user" },
            { id: "u-other", display_name: "Lena Fischer", kind: "user" },
          ],
          page: { next_cursor: null },
        });
      }
      if (request.method === "PATCH" && pathname.includes("/leads/l-1")) {
        return onPatch(JSON.parse(await request.text()));
      }
      if (pathname.endsWith("/v1/leads/settings")) {
        return jsonResponse({ first_response_enabled: false });
      }
      if (pathname.includes("/history") || pathname.includes("/activities")) {
        return jsonResponse({ data: [], page: { next_cursor: null } });
      }
      return jsonResponse(lead);
    }),
  );
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>{ui}</RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

/** Start an override from the score card's head. */
async function openOverride(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", { name: "Override score" }),
  );
}

describe("the lead page's writes, read from two columns", () => {
  it("keeps a half-typed override when a DIFFERENT control's write lands", async () => {
    const user = userEvent.setup();
    stubFetch((body) =>
      jsonResponse({ ...lead, ...(body as object), version: 2 }),
    );
    render(<LeadScreen id="l-1" />);

    await openOverride(user);
    await user.type(
      await screen.findByRole("spinbutton", { name: "Score" }),
      "90",
    );
    await user.type(screen.getByLabelText("Reason"), "Met the buyer");

    // Somebody else's write: the owner picker in the rail beside this form.
    await changeChoice(user, "Owner", "Assign to me");

    // The override survives it.
    await waitFor(() =>
      expect(
        (
          screen.getByRole("spinbutton", {
            name: "Score",
          }) as HTMLInputElement
        ).value,
      ).toBe("90"),
    );
    expect((screen.getByLabelText("Reason") as HTMLInputElement).value).toBe(
      "Met the buyer",
    );
  });

  it("states a refused owner write once, in its dialog, on the History tab", async () => {
    const user = userEvent.setup();
    stubFetch(() =>
      jsonResponse(
        {
          type: "about:blank",
          title: "Conflict",
          status: 409,
          detail: "This lead moved while you were reading it.",
        },
        409,
      ),
    );
    render(<LeadScreen id="l-1" />);

    // The rail renders on every tab, so its writes can be refused on one the
    // ladder panel — where this sentence used to live — never reaches.
    await user.click(await screen.findByRole("button", { name: /^History/ }));
    await changeChoice(user, "Owner", "Assign to me");

    const dialog = await screen.findByRole("dialog", { name: "Owner" });
    await waitFor(() =>
      expect(within(dialog).getByRole("alert").textContent).toMatch(
        /moved while you were reading it/,
      ),
    );
    expect(
      screen.getAllByRole("alert").filter((alert) => !dialog.contains(alert)),
    ).toEqual([]);

    // Cancel leaves no copy of the refusal behind on the page.
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.queryByText(/moved while you were reading it/)).toBeNull();
  });
});
