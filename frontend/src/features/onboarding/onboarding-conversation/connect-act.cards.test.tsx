/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../../i18n/en";
import { jsonResponse, stubWithSession } from "../../../testing/story-utils";
import { renderConnectAct, rosterWith } from "./connect-act.testkit";

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  // The OAuth-attempt mark lives in sessionStorage (see
  // onboarding-connect-panels.tsx). jsdom shares it across tests in one
  // process, so a mark one test writes must not leak into the next.
  sessionStorage.clear();
});

// LinkedIn lives beside mail on the same screen. Saving or skipping it never
// touches the mail flow above. Neither dispatch is CONNECT_DONE or a
// wizard-state write, because the mail gate owns those. It is not a
// connection: the member writes down which profile their imported network is
// attributed to, and nothing is authorized.
describe("the LinkedIn card", () => {
  const saves: unknown[] = [];
  beforeEach(() => {
    saves.length = 0;
    stubWithSession(
      {
        "GET /connectors": () => jsonResponse({ data: [] }),
        "PUT /me/linkedin-account": (body: unknown) => {
          saves.push(body);
          return jsonResponse({ connected: false, connections: 0 });
        },
      },
      {},
    );
  });

  it("keeps the profile form closed until its card is clicked", () => {
    renderConnectAct();
    expect(screen.getByText("Save")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Save profile" })).toBeNull();
  });

  it("saves the profile the network is attributed to, and authorizes nothing", async () => {
    const { dispatch } = renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: /LinkedIn/ }));

    // The ask opens as its own dialog, same as a mail provider's, and says
    // where the connections themselves come from.
    expect(await screen.findByRole("dialog")).toBeTruthy();
    expect(screen.getByText(en["ob.conv.linkedin.importLater"])).toBeTruthy();
    expect(screen.queryByRole("button", { name: /authori[sz]e/i })).toBeNull();
    const button = screen.getByRole("button", { name: "Save profile" });
    expect(button).toBeDisabled();

    await userEvent.type(
      screen.getByLabelText("Your LinkedIn profile URL"),
      "https://www.linkedin.com/in/lars",
    );
    expect(button).not.toBeDisabled();
    await userEvent.click(button);

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({
        type: "LINKEDIN_SAVED",
        profile: "https://www.linkedin.com/in/lars",
      }),
    );
    // `connected: false` on the wire: a saved address grants nothing, and the
    // store keeps whatever authorization already exists.
    expect(saves).toEqual([
      { profile_url: "https://www.linkedin.com/in/lars", connected: false },
    ]);
  });

  it("can be declined from the open dialog, without a profile", async () => {
    const { dispatch } = renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: /LinkedIn/ }));
    await userEvent.click(
      screen.getByRole("button", { name: "Skip LinkedIn for now" }),
    );
    expect(dispatch).toHaveBeenCalledWith({ type: "LINKEDIN_SKIPPED" });
  });

  it("shows the resolved state and offers no further action once skipped", () => {
    renderConnectAct(undefined, "skipped");
    expect(screen.getByText("Skipped: add it later in Settings")).toBeTruthy();
    expect(screen.getByRole("button", { name: /LinkedIn/ })).toBeDisabled();
  });

  it("shows the resolved state and offers no further action once connected", () => {
    renderConnectAct(undefined, "saved");
    expect(screen.getByText("Profile saved")).toBeTruthy();
    // A saved address is not a connected integration, and the tile's own
    // affordance must not call it one.
    expect(screen.getByText("Saved")).toBeTruthy();
    expect(screen.queryByText("connected")).toBeNull();
    expect(screen.getByRole("button", { name: /LinkedIn/ })).toBeDisabled();
  });

  // A failed save must stay visible and retryable, not vanish behind a
  // dialog that already closed on the click that failed.
  it("keeps the dialog open and shows the failure when the save fails", async () => {
    stubWithSession(
      {
        "GET /connectors": () => jsonResponse({ data: [] }),
        "PUT /me/linkedin-account": () =>
          jsonResponse({ detail: "LinkedIn refused the profile." }, 422),
      },
      {},
    );
    renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: /LinkedIn/ }));
    await userEvent.type(
      screen.getByLabelText("Your LinkedIn profile URL"),
      "https://www.linkedin.com/in/lars",
    );
    await userEvent.click(screen.getByRole("button", { name: "Save profile" }));

    expect(
      await screen.findByText("LinkedIn refused the profile."),
    ).toBeInTheDocument();
    // Still open and retryable, not dismissed on the failed click.
    expect(
      screen.getByRole("button", { name: "Save profile" }),
    ).toBeInTheDocument();
  });
});

// The way on is read and acted on in one place. That place is the work
// surface the reader has been looking at, not a chip beside the transcript.
describe("the way onward", () => {
  it("renders on the stage's rail — never as a thread chip", () => {
    stubWithSession(
      { "GET /connectors": () => jsonResponse({ data: [] }) },
      {},
    );
    renderConnectAct();
    const onward = screen.getByRole("button", { name: "Continue" });
    expect(onward.closest(".ob-stage-acts")).toBeTruthy();
  });

  // With a mailbox live the step is left as connected: the skip flag is
  // false, and leaving the last stop is what writes completion.
  it("records the step as connected and hands on when a mailbox is live", async () => {
    stubWithSession({ "GET /connectors": rosterWith({ state: "none" }) }, {});
    const { dispatch, persist } = renderConnectAct();
    const onward = screen.getByRole("button", { name: "Continue" });
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Continue without a mailbox" }),
      ).toBeNull(),
    );
    await userEvent.click(onward);
    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "CONNECT_DONE" }),
    );
    expect(persist).toHaveBeenCalledWith(
      expect.objectContaining({ step: "complete", connectSkipped: false }),
    );
  });
});

// The four step-level consent guarantees render on the artifact surface,
// where the reader passes through them before authorizing anything. The rail's
// narrow column would wrap them into broken-looking text. They sit behind a
// fold that names them, like every other disclosure in the product. They are
// one click from the scene, not inside a provider's dialog.
describe("the consent guarantees", () => {
  it("sit on the surface behind a named fold, not in the rail", async () => {
    stubWithSession(
      { "GET /connectors": () => jsonResponse({ data: [] }) },
      {},
    );
    renderConnectAct();

    const toggle = screen.getByText(en["ob.conv.connect.guaranteesToggle"]);
    const fold = toggle.closest("details");
    expect(fold?.open).toBe(false);
    await userEvent.click(toggle);
    expect(fold?.open).toBe(true);

    for (const key of [
      "ob.s4.scope1Lead",
      "ob.s4.scope2Lead",
      "ob.s4.scope3Lead",
      "ob.s4.scope4Lead",
    ] as const) {
      const found = screen.getByText(en[key]);
      expect(found.closest("details")).toBe(fold);
    }
  });
});

describe("the IMAP dialog", () => {
  it("carries only the real contract's fields — no invented SMTP host, port or TLS toggle", async () => {
    stubWithSession(
      { "GET /connectors": () => jsonResponse({ data: [] }) },
      {},
    );
    renderConnectAct();
    await userEvent.click(
      screen.getByRole("button", { name: /Other mailbox/ }),
    );

    const dialog = await screen.findByRole("dialog");
    expect(screen.getByLabelText("IMAP host")).toBeTruthy();
    expect(screen.getByLabelText("Port")).toBeTruthy();
    expect(screen.getByLabelText("Email")).toBeTruthy();
    expect(screen.getByLabelText("App password")).toBeTruthy();
    expect(screen.getByLabelText("Mailbox")).toBeTruthy();
    // The prototype this dialog adapts shows SMTP host/port and a "Require
    // TLS" checkbox. The connector contract has neither, so this form carries
    // no checkbox and no widget that would submit nothing.
    //
    // Scoped to the dialog, which is what the claim is about. The scene behind
    // it carries the overnight question's own checkbox, and a document-wide
    // count would read that as this form having grown a widget.
    expect(within(dialog).queryByLabelText(/smtp/i)).toBeNull();
    expect(within(dialog).queryAllByRole("checkbox")).toHaveLength(0);
  });

  it("closes on 'Not now' without touching the required-step skip", async () => {
    stubWithSession(
      { "GET /connectors": () => jsonResponse({ data: [] }) },
      {},
    );
    const { dispatch, persist } = renderConnectAct();
    await userEvent.click(
      screen.getByRole("button", { name: /Other mailbox/ }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Not now" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    // Backing out of one provider's ask is not the same decision as skipping
    // the whole required mailbox step, so neither fires here.
    expect(dispatch).not.toHaveBeenCalled();
    expect(persist).not.toHaveBeenCalled();
    // The card grid is still there, ready for a different pick.
    expect(screen.getByRole("button", { name: /Microsoft/ })).toBeTruthy();
  });
});

// The disabled "Not now"/"Skip" buttons guard only their own click. The
// dialog's other dismissal routes (X, Escape, backdrop) resolve to one close
// handler (`ConnectDialog` → `Modal`). A success landing after the reader
// backed out would store a connection against the panel's promise. Each
// provider keeps that handler from acting while its own save is in flight.
describe("dismissal during an in-flight connect request", () => {
  it("keeps the OAuth dialog open against Escape while its connect POST is pending", async () => {
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    stubWithSession(
      {
        "GET /connectors": () => jsonResponse({ data: [] }),
        "POST /connectors/graph/connect": () =>
          new Promise((resolve) => {
            deferred.resolve = resolve;
          }),
      },
      {},
    );
    renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: /Microsoft/ }));
    await userEvent.click(
      screen.getByRole("button", {
        name: "Connect Microsoft",
      }),
    );
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Not now" })).toBeDisabled(),
    );

    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeTruthy();

    deferred.resolve?.(jsonResponse({}));
  });

  it("keeps the IMAP dialog open against Escape while its connect POST is pending", async () => {
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    stubWithSession(
      {
        "GET /connectors": () => jsonResponse({ data: [] }),
        "POST /connectors/imap/connect": () =>
          new Promise((resolve) => {
            deferred.resolve = resolve;
          }),
      },
      {},
    );
    renderConnectAct();
    await userEvent.click(
      screen.getByRole("button", { name: /Other mailbox/ }),
    );
    await userEvent.type(screen.getByLabelText("Email"), "me@example.com");
    await userEvent.type(screen.getByLabelText("App password"), "secret");
    await userEvent.click(
      screen.getByRole("button", { name: "Test and connect" }),
    );
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Not now" })).toBeDisabled(),
    );

    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeTruthy();

    deferred.resolve?.(
      jsonResponse({
        connection: {
          id: "c1",
          provider: "imap",
          status: "connected",
          scopes: [],
        },
      }),
    );
    await screen.findByText(/mailbox connected/i);
  });

  // LinkedIn's skip and its save race the same way mail's dismiss does. A
  // PUT landing after a skip dispatched would save the profile against a
  // machine state that says skipped. The later LINKEDIN_SAVED dispatch could
  // not be told apart from a stale one.
  it("keeps the LinkedIn dialog open and Skip disabled while its save is pending", async () => {
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    stubWithSession(
      {
        "GET /connectors": () => jsonResponse({ data: [] }),
        "PUT /me/linkedin-account": () =>
          new Promise((resolve) => {
            deferred.resolve = resolve;
          }),
      },
      {},
    );
    const { dispatch } = renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: /LinkedIn/ }));
    await userEvent.type(
      screen.getByLabelText("Your LinkedIn profile URL"),
      "https://www.linkedin.com/in/lars",
    );
    await userEvent.click(screen.getByRole("button", { name: "Save profile" }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Skip LinkedIn for now" }),
      ).toBeDisabled(),
    );

    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(dispatch).not.toHaveBeenCalledWith({ type: "LINKEDIN_SKIPPED" });

    deferred.resolve?.(jsonResponse({ connected: false, connections: 0 }));
    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({
        type: "LINKEDIN_SAVED",
        profile: "https://www.linkedin.com/in/lars",
      }),
    );
  });
});
