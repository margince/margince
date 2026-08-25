/** @vitest-environment jsdom */
import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createQueryClient } from "../../app/queryclient";
import {
  LINKEDIN_ACCOUNT_KEY,
  useSaveLinkedInAccount,
} from "./use-linkedin-account";

// The onboarding act and the settings row save the SAME account through this
// one hook. Two things used to depend on which copy a caller happened to reach:
// whether the member's authorization could travel at all, and whether the
// settings tab saw the save. Both are pinned here.
//
// The application's own client, not a bare one, so the cache these assertions
// read is the cache the screens read (app/queryclient.ts).

const SAVED = {
  profile_url: "https://linkedin.com/in/lars",
  connected: true,
};

afterEach(() => {
  vi.unstubAllGlobals();
});

function renderSave() {
  const client = createQueryClient();
  function wrapper({ children }: Readonly<{ children: ReactNode }>) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  }
  return { client, ...renderHook(() => useSaveLinkedInAccount(), { wrapper }) };
}

describe("useSaveLinkedInAccount", () => {
  it("sends the authorization it was given rather than a constant", async () => {
    // The generated client sends a Request, so the body is read off that
    // rather than off an init object it never passes.
    let sentBody: unknown = null;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (input instanceof Request) {
          sentBody = await input.clone().json();
        }
        return Response.json(SAVED);
      }),
    );
    const { result } = renderSave();

    act(() => {
      result.current.mutate({
        profileUrl: SAVED.profile_url,
        connected: true,
      });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(sentBody).toEqual({
      profile_url: SAVED.profile_url,
      connected: true,
    });
  });

  it("writes the saved account into the cache the settings row reads", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json(SAVED)),
    );
    const { client, result } = renderSave();

    act(() => {
      result.current.mutate({
        profileUrl: SAVED.profile_url,
        connected: true,
      });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    // Under the READ's key, not merely somewhere: a save that landed under a
    // key nobody watches leaves the settings tab showing the old profile until
    // a reload, which is the drift the second copy of this hook had.
    expect(client.getQueryData(LINKEDIN_ACCOUNT_KEY)).toEqual(SAVED);
  });
});
