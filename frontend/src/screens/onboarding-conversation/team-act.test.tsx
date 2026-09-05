// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../../app/mefixture";
import { LocaleProvider } from "../../i18n";
import { installFetchStub, jsonResponse } from "../story-utils";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";
import { TeamAct } from "./team-act";

// The team act invites through the settings form and then finishes. Two
// things it must get right: the write that closes the journey lands before
// the dispatch that plays the handoff (and a refused write keeps the reader
// here), and an invite on an installation with no email channel hands the
// admin the set-password link straight away.

const asking: ConversationState = {
  ...initialConversationState,
  act: "team",
  phase: "tm.ask",
};

const NEW_USER = "018f3a1b-0000-7000-8000-0000000000e7";

function renderTeam(
  options: { passwordLink?: boolean; persisted?: boolean } = {},
) {
  installFetchStub({
    // An admin, on an installation that may or may not be able to mail: the
    // server says which through `admin_password_link`.
    "GET /me": () =>
      jsonResponse({
        ...meFixture({ allow: {} }),
        admin_password_link: options.passwordLink ?? false,
      }),
    "GET /teams": () => jsonResponse({ data: [], next_cursor: null }),
    "POST /users/access-preview": () =>
      jsonResponse({ role: "rep", row_scope: "own", objects: {} }),
    "POST /users": () => jsonResponse({ id: NEW_USER }, 201),
    [`POST /users/${NEW_USER}/password-link`]: () =>
      jsonResponse({
        set_password_url: "https://crm.example/set-password?t=abc",
        expires_at: "2026-09-04T12:00:00Z",
      }),
  });
  const dispatch = vi.fn();
  const persist = vi.fn(async () => options.persisted ?? true);
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <TeamAct state={asking} dispatch={dispatch} persist={persist} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { dispatch, persist };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("TeamAct", () => {
  it("shows the settings invite form under its own title, and a skip while nobody is invited", async () => {
    renderTeam();
    expect(
      await screen.findByText("Invite the first user."),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/New user's email/)).toBeInTheDocument();
    // One address is enough here; the name is derived from it.
    expect(screen.queryByLabelText(/New user's full name/)).toBeNull();
    // The form's own heading stays in the settings dialog: here the scene's
    // title has already said it.
    expect(screen.queryByText("Invite a user")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Skip for now" }),
    ).toBeInTheDocument();
  });

  it("skipping closes the journey: completion is written, then the act hands on", async () => {
    const { dispatch, persist } = renderTeam();
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", { name: "Skip for now" }),
    );

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "TEAM_DONE" }),
    );
    expect(persist).toHaveBeenCalledWith(
      expect.objectContaining({ step: "complete" }),
    );
  });

  it("says so and stays when completion could not be written", async () => {
    const { dispatch, persist } = renderTeam({ persisted: false });
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", { name: "Skip for now" }),
    );

    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(persist).toHaveBeenCalledTimes(1);
    expect(dispatch).not.toHaveBeenCalled();
  });

  it("an invite lists the person, turns the skip into a finish, and mints their link where no mail can carry it", async () => {
    renderTeam({ passwordLink: true });
    const user = userEvent.setup();

    await user.type(
      await screen.findByLabelText(/New user's email/),
      "ada.byron@example.com",
    );
    await user.click(screen.getByRole("button", { name: "Invite" }));

    expect(
      await screen.findByText("Ada Byron is invited."),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Finish setup" }),
    ).toBeInTheDocument();
    // The set-password link, in the same dialog the roster hands it over in.
    expect(
      await screen.findByDisplayValue("https://crm.example/set-password?t=abc"),
    ).toBeInTheDocument();
  });
});
