// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { installFetchStub, meRoute } from "../story-utils";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";
import { InviteAct } from "./invite-act";

// The invite decides whether the personal steps happen at all, and the two
// answers are not symmetrical: yes only moves the conversation, no is a
// finish that has to be RECORDED before anything else happens.

const asking: ConversationState = {
  ...initialConversationState,
  act: "invite",
  phase: "in.ask",
};

function renderInvite(persist: (input: unknown) => Promise<boolean>) {
  installFetchStub({ "GET /me": meRoute({}) });
  const dispatch = vi.fn();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <InviteAct state={asking} dispatch={dispatch} persist={persist} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return dispatch;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("InviteAct", () => {
  it("asks the question with what each answer opens, and offers both", async () => {
    renderInvite(vi.fn(async () => true));
    expect(
      await screen.findByText("Will you be working in Margince yourself?"),
    ).toBeInTheDocument();
    expect(screen.getByText(/Train your voice/)).toBeInTheDocument();
    expect(
      screen.getByText(/Connect your inbox and calendar/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Yes, set me up" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "No, I'm only setting it up" }),
    ).toBeInTheDocument();
  });

  it("accepting only moves the conversation: nothing is written yet", async () => {
    const persist = vi.fn(async () => true);
    const dispatch = renderInvite(persist);
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", { name: "Yes, set me up" }),
    );

    expect(dispatch).toHaveBeenCalledWith({ type: "INVITE_ACCEPTED" });
    expect(persist).not.toHaveBeenCalled();
  });

  it("declining records completion with both personal steps skipped, then ends the journey", async () => {
    const persist = vi.fn(async () => true);
    const dispatch = renderInvite(persist);
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", {
        name: "No, I'm only setting it up",
      }),
    );

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "INVITE_DECLINED" }),
    );
    expect(persist).toHaveBeenCalledWith(
      expect.objectContaining({
        step: "complete",
        voiceSkipped: true,
        connectSkipped: true,
      }),
    );
    // The write landed before the dispatch, never after it.
    expect(persist.mock.invocationCallOrder[0]).toBeLessThan(
      dispatch.mock.invocationCallOrder[0],
    );
  });

  it("a decline that could not be recorded stays on the question and says so", async () => {
    const persist = vi.fn(async () => false);
    const dispatch = renderInvite(persist);
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", {
        name: "No, I'm only setting it up",
      }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /couldn't record that setup is complete/,
    );
    expect(dispatch).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: "No, I'm only setting it up" }),
    ).toBeEnabled();
  });
});
