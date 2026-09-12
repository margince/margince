/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { SettingsScreen } from "./settings";
import { IDLE_JOB_HEALTH, jsonResponse, render } from "./settings.testkit";
import { settingsHref } from "./settingsrouting";

// The danger zone on the Maintenance entry. Reset data is the one control on
// this screen that destroys an installation's data, so it is gated twice —
// `system_reset:delete`, which `POST /admin/reset-data` asks for, AND the
// `data_reset_available` switch a deployment arms — and it owes the reader who
// ran it a report of what it actually cleared.

// No shared fetch stub: the backend a claim needs is installed beside the claim,
// so what answered it is readable where it is asserted.
beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

// The danger-zone Reset data action: server-driven, gated on
// `system_reset:delete` AND me.data_reset_available — the switch a deployment
// arms, not the posture it happens to run under. A dedicated backend per test
// so the grant/capability combination is explicit rather than layered on the
// shared default.
//
// `allow` defaults to `system_reset:delete`, the grant that opens the page this
// card now has to itself — a test about the card should not also have to argue
// its way onto the page, and one that wants it CLOSED says so with `{}`. It was
// the reindex write while the card sat on a combined Maintenance entry; the
// reindex and the job report are their own page now.
function resetDataBackend(opts: {
  roles: string[];
  dataResetAvailable: boolean;
  allow?: GrantSpec;
  onReset?: (body: unknown) => void;
  resetStatus?: number;
  resetBody?: unknown;
}) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input instanceof Request ? input.url : input);
    const method = (
      input instanceof Request ? input.method : (init?.method ?? "GET")
    ).toUpperCase();
    if (url.endsWith("/v1/me")) {
      const me = meFixture({
        roles: opts.roles,
        allow: opts.allow ?? { system_reset: ["delete"] },
      });
      return jsonResponse({
        ...me,
        user: { ...me.user, email: "ada@acme.test" },
        workspace_name: "Acme Inc",
        data_reset_available: opts.dataResetAvailable,
      });
    }
    // The job report is the danger zone's neighbour on this entry, and a
    // `job_health:read` holder fetches it on arrival — so it answers with the shape the endpoint serves.
    // A generic `{data: []}` here would crash the card that reads it, and every
    // assertion below would fail describing the wrong thing.
    if (url.includes("/admin/job-health")) {
      return jsonResponse(IDLE_JOB_HEALTH);
    }
    if (url.includes("/admin/reset-data") && method === "POST") {
      const raw = input instanceof Request ? await input.clone().text() : "";
      const body = raw ? JSON.parse(raw) : {};
      opts.onReset?.(body);
      if (opts.resetStatus && opts.resetStatus !== 200) {
        return jsonResponse(
          opts.resetBody ?? { detail: "confirmation mismatch" },
          opts.resetStatus,
        );
      }
      return jsonResponse(
        opts.resetBody ?? { status: "reset", tables_cleared: 3 },
      );
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

describe("ResetDataCard (danger zone)", () => {
  it("shows the Reset data control for an admin where the reset is armed", async () => {
    vi.stubGlobal(
      "fetch",
      resetDataBackend({ roles: ["admin"], dataResetAvailable: true }),
    );
    render(<SettingsScreen route={settingsHref("reset")} />);
    expect(await screen.findByText(/reset data/i)).toBeTruthy();
  });

  it("hides Reset data for an admin where the reset was never armed", async () => {
    vi.stubGlobal(
      "fetch",
      resetDataBackend({ roles: ["admin"], dataResetAvailable: false }),
    );
    render(<SettingsScreen route={settingsHref("reset")} />);
    // A STRONGER answer than the card hiding itself, and it is the split that
    // buys it: the danger zone had to sit unarmed on a page a reader was
    // already on, so the page rendered and the card withheld itself. It has a
    // page of its own now, so an unarmed installation has no such destination —
    // the address reaches the boundary and nothing about resetting appears.
    expect(
      await screen.findByText(/this settings page is not yours to open/i),
    ).toBeTruthy();
    expect(screen.queryByText(/reset data/i)).toBeNull();
  });

  it("hides Reset data from a rep even where the reset is armed", async () => {
    vi.stubGlobal(
      "fetch",
      // A rep holds no `system_reset` grant, so the page is not theirs to
      // reach in the first place.
      resetDataBackend({ roles: ["rep"], dataResetAvailable: true, allow: {} }),
    );
    render(<SettingsScreen route={settingsHref("reset")} />);
    // With no grant the rep is told the page is not theirs, and the address
    // they were sent is left in the bar. Waited on the boundary's own words
    // rather than on an absence, which is also true mid-load.
    expect(
      await screen.findByText(/this settings page is not yours to open/i),
    ).toBeTruthy();
    expect(screen.queryByText(/reset data/i)).toBeNull();
  });

  // `system_reset:delete` is the whole gate, on the page AND on the card inside
  // it: `POST /admin/reset-data` asks for that grant (compose/datareset.go),
  // and `ResetDataCard` asks the same object where it used to ask whether the
  // reader WAS an admin. So a role EDITED to carry the verb reaches the
  // control — which the role name could not express either way.
  it("shows the control to an ops role edited to carry the delete verb", async () => {
    vi.stubGlobal(
      "fetch",
      resetDataBackend({
        roles: ["ops"],
        dataResetAvailable: true,
        allow: { system_reset: ["delete"] },
      }),
    );
    render(<SettingsScreen route={settingsHref("reset")} />);
    expect(
      await screen.findByRole("button", { name: /reset data/i }),
    ).toBeTruthy();
  });

  // The other half, or the case above would pass against a card that showed the
  // control to everyone: an ops seat WITHOUT the verb reaches nothing, however
  // armed the installation is.
  it("withholds the control from an ops seat without the delete verb", async () => {
    vi.stubGlobal(
      "fetch",
      resetDataBackend({
        roles: ["ops"],
        dataResetAvailable: true,
        allow: { contact: ["read"] },
      }),
    );
    render(<SettingsScreen route={settingsHref("reset")} />);
    // The catalog gives this reader no reset page at all, so the address
    // reaches the boundary — asserted through its own words rather than through
    // an absence that is also true mid-load.
    expect(
      await screen.findByText(/this settings page is not yours to open/i),
    ).toBeTruthy();
    expect(screen.queryByText(/reset data/i)).toBeNull();
  });

  it("enables the confirm button once the input is non-empty and POSTs the typed confirmation", async () => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      resetDataBackend({
        roles: ["admin"],
        dataResetAvailable: true,
        onReset: (body) => posted.push(body),
      }),
    );
    render(<SettingsScreen route={settingsHref("reset")} />);
    await user.click(
      await screen.findByRole("button", { name: /reset data/i }),
    );

    const dialog = await screen.findByRole("dialog");
    // The company name is shown so the admin can copy it into the input.
    expect(within(dialog).getByText("Acme Inc")).toBeTruthy();
    const confirmButton = within(dialog).getByRole("button", {
      name: /reset everything/i,
    });
    expect(confirmButton).toHaveProperty("disabled", true);

    const input = within(dialog).getByRole("textbox");
    await user.type(input, "Acme Inc");
    expect(confirmButton).toHaveProperty("disabled", false);

    await user.click(confirmButton);

    await waitFor(() => expect(posted).toEqual([{ confirmation: "Acme Inc" }]));
    // The dialog closes and the input clears on success.
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("surfaces the server's confirmation-mismatch message on a 422", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      resetDataBackend({
        roles: ["admin"],
        dataResetAvailable: true,
        resetStatus: 422,
        resetBody: {
          detail: "The typed confirmation does not match the company name.",
        },
      }),
    );
    render(<SettingsScreen route={settingsHref("reset")} />);
    await user.click(
      await screen.findByRole("button", { name: /reset data/i }),
    );
    const dialog = await screen.findByRole("dialog");
    const input = within(dialog).getByRole("textbox");
    await user.type(input, "Wrong Name");
    await user.click(
      within(dialog).getByRole("button", { name: /reset everything/i }),
    );
    expect(
      await screen.findByText(
        "The typed confirmation does not match the company name.",
      ),
    ).toBeTruthy();
  });

  // The full response (Task 8's five extra counters) — an admin who triggers a
  // reset that now spans tables, jobs, streams, cache keys and blob storage
  // learns what actually happened, not just that the button worked.
  const fullResetBody = {
    status: "reset",
    tables_cleared: 84,
    jobs_deleted: 12,
    streams_purged: 12,
    cache_keys_deleted: 341,
    objects_deleted: 7,
    drain_timed_out: false,
  };

  // Fixed precondition for every summary test below: admin + the armed reset,
  // on the Maintenance entry that hosts the card.
  function renderSettingsAsAdmin(opts: { resetResponse: unknown }) {
    vi.stubGlobal(
      "fetch",
      resetDataBackend({
        roles: ["admin"],
        dataResetAvailable: true,
        resetBody: opts.resetResponse,
      }),
    );
    return render(<SettingsScreen route={settingsHref("reset")} />);
  }

  // Opens the confirm dialog, types the confirmation, and submits — the same
  // three steps every summary test needs before it can see a result.
  async function confirmReset(user: UserEvent, companyName: string) {
    await user.click(
      await screen.findByRole("button", { name: /reset data/i }),
    );
    const dialog = await screen.findByRole("dialog");
    const input = within(dialog).getByRole("textbox");
    await user.type(input, companyName);
    await user.click(
      within(dialog).getByRole("button", { name: /reset everything/i }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  }

  it("reports what the reset cleared", async () => {
    const user = userEvent.setup();
    renderSettingsAsAdmin({ resetResponse: fullResetBody });
    await confirmReset(user, "Acme Inc");

    expect(
      await screen.findByText(
        // The whole line, not a prefix: dropping the trailing counters is
        // exactly the regression this guards, and a prefix match would pass.
        "Cleared 84 tables, 12 job rows, 12 event streams, 341 cache keys and 7 stored files.",
      ),
    ).toBeInTheDocument();
  });

  it("warns when a job was still running at drain time", async () => {
    const user = userEvent.setup();
    renderSettingsAsAdmin({
      resetResponse: { ...fullResetBody, drain_timed_out: true },
    });
    await confirmReset(user, "Acme Inc");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /background job was still running/,
    );
  });

  it("shows no summary before a reset has run", async () => {
    renderSettingsAsAdmin({ resetResponse: fullResetBody });
    // Wait for the card itself: until /v1/me resolves ResetDataCard renders
    // null, and an assertion made before that passes against an empty screen
    // rather than against a card that is deliberately quiet.
    const card = (
      await screen.findByRole("button", { name: /reset data/i })
    ).closest("section");
    if (!(card instanceof HTMLElement)) {
      throw new Error("the Reset data control renders outside a card");
    }
    // Read that card rather than the page: the summary is a status region inside
    // it, and the two cards above it on Maintenance each render a loading
    // skeleton that is also a status region while its query is in flight — a
    // page-wide query would be answered by whichever of those was still pending.
    expect(within(card).queryByRole("status")).not.toBeInTheDocument();
  });

  it("clears a prior success summary once a retry fails, rather than showing both", async () => {
    const user = userEvent.setup();
    // The first POST to /admin/reset-data succeeds; the second (a retry, e.g.
    // after a typo) 422s. A dedicated fetch mock rather than resetDataBackend
    // because that helper's resetStatus is fixed for every call — this test
    // needs the response to change between the two attempts.
    let resetCalls = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input instanceof Request ? input.url : input);
        const method = (
          input instanceof Request ? input.method : (init?.method ?? "GET")
        ).toUpperCase();
        if (url.endsWith("/v1/me")) {
          const me = meFixture({
            roles: ["admin"],
            allow: { system_reset: ["delete"] },
          });
          return jsonResponse({
            ...me,
            workspace_name: "Acme Inc",
            data_reset_available: true,
          });
        }
        if (url.includes("/admin/job-health")) {
          return jsonResponse(IDLE_JOB_HEALTH);
        }
        if (url.includes("/admin/reset-data") && method === "POST") {
          resetCalls += 1;
          if (resetCalls === 1) {
            return jsonResponse(fullResetBody);
          }
          return jsonResponse(
            { detail: "The typed confirmation does not match." },
            422,
          );
        }
        return jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        });
      }),
    );
    render(<SettingsScreen route={settingsHref("reset")} />);

    await confirmReset(user, "Acme Inc");
    expect(
      await screen.findByText(/Cleared 84 tables, 12 job rows/),
    ).toBeInTheDocument();

    // Retry: the dialog stays open on error, so the summary from the first
    // attempt must not still be sitting behind it.
    await user.click(
      await screen.findByRole("button", { name: /reset data/i }),
    );
    const dialog = await screen.findByRole("dialog");
    const input = within(dialog).getByRole("textbox");
    await user.type(input, "Acme Inc");
    await user.click(
      within(dialog).getByRole("button", { name: /reset everything/i }),
    );

    expect(
      await screen.findByText("The typed confirmation does not match."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});
