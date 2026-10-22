import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render } from "@testing-library/react";
import { type Mock, vi } from "vitest";
import { LocaleProvider } from "../../../i18n";
import { jsonResponse } from "../../../testing/story-utils";
import { ConnectAct } from "./connect-act";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

export function renderConnectAct(
  outcome?: string,
  linkedinStatus: ConversationState["linkedinStatus"] = "pending",
  phase: ConversationState["phase"] = "cn.consent",
  returningProvider?: string,
): RenderResult & { dispatch: Mock; persist: Mock } {
  const state: ConversationState = {
    ...initialConversationState,
    act: "connect",
    phase,
    linkedinStatus,
  };
  const dispatch = vi.fn();
  const persist = vi.fn(async () => true);
  const view = render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ConnectAct
          state={state}
          dispatch={dispatch}
          persist={persist}
          outcome={outcome}
          returningProvider={returningProvider}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { ...view, dispatch, persist };
}

export const rosterWith = (backfill: Record<string, unknown>) => () =>
  jsonResponse({
    data: [
      {
        id: "g1",
        provider: "gmail",
        status: "connected",
        scopes: ["read"],
        backfill,
      },
    ],
  });
