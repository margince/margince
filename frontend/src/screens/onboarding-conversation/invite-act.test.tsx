// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { installFetchStub, meRoute } from "../story-utils";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";
import { InviteAct } from "./invite-act";

// The invite decides whether the personal steps happen at all. It is one
// question with two answers and one Continue: the act writes nothing itself,
// it only says which way the conversation goes.

const asking: ConversationState = {
  ...initialConversationState,
  act: "invite",
  phase: "in.ask",
};

function renderInvite() {
  installFetchStub({ "GET /me": meRoute({}) });
  const dispatch = vi.fn();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <InviteAct state={asking} dispatch={dispatch} />
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
  it("asks the question as two answers, and holds Continue until one is picked", async () => {
    renderInvite();
    expect(
      await screen.findByRole("heading", {
        name: "Will you be working in Margince yourself?",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("radio", { name: /Yes, I'll work in Margince/ }),
    ).not.toBeChecked();
    expect(
      screen.getByRole("radio", { name: /No, I'm only setting it up/ }),
    ).not.toBeChecked();
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
  });

  it("yes opens the personal steps", async () => {
    const dispatch = renderInvite();
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("radio", { name: /Yes, I'll work in Margince/ }),
    );
    await user.click(screen.getByRole("button", { name: "Continue" }));

    expect(dispatch).toHaveBeenCalledWith({ type: "INVITE_ACCEPTED" });
  });

  it("no opens the team act instead", async () => {
    const dispatch = renderInvite();
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("radio", { name: /No, I'm only setting it up/ }),
    );
    await user.click(screen.getByRole("button", { name: "Continue" }));

    expect(dispatch).toHaveBeenCalledWith({ type: "INVITE_DECLINED" });
    expect(dispatch).not.toHaveBeenCalledWith({ type: "INVITE_ACCEPTED" });
  });
});
