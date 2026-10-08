/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render,
  renderHook,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { LEAD_LIST_KEY } from "./leadkeys";
import { LeadsScreen } from "./leads";
import { jsonResponse } from "./story-utils";
import { useApplyTag } from "./tags.queries";

const TAG_A = "01a114b9-2725-7389-967d-a0ef76e58930";
const TAG_B = "01a114b9-2725-7389-967d-a0ef76e58931";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("a lead's tags", () => {
  it("reach the lead list one id per parameter, as the endpoint reads them", async () => {
    const urls: string[] = [];
    vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.includes("/me")) {
        return jsonResponse({
          user: { id: "u-me", email: "me@example.test", display_name: "Me" },
          roles: ["rep"],
          teams: [],
          authorization: meFixture({
            allow: { lead: ["read"], tag: ["read"] },
          }).authorization,
        });
      }
      if (/\/leads\?/.test(url)) {
        urls.push(url);
      }
      return jsonResponse({
        data: [],
        page: { next_cursor: null, has_more: false, total: 0 },
      });
    });
    window.location.hash = `#/leads?tag_id=${TAG_A},${TAG_B}`;
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <LeadsScreen />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    await waitFor(() => expect(urls.length).toBeGreaterThan(0));
    const sent = new URL(urls.at(-1) ?? "", "http://localhost").searchParams;
    expect(sent.getAll("tag_id")).toEqual([TAG_A, TAG_B]);
  });

  it("leave the lead list stale once one is applied, so its chips redraw", async () => {
    vi.stubGlobal("fetch", async () => jsonResponse({}, 201));
    const client = new QueryClient();
    client.setQueryData([...LEAD_LIST_KEY, { q: "" }], { data: [] });
    const wrapper = ({ children }: Readonly<{ children: ReactNode }>) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useApplyTag("lead", "l-1"), {
      wrapper,
    });

    await act(() => result.current.mutateAsync(TAG_A));

    expect(
      client.getQueryState([...LEAD_LIST_KEY, { q: "" }])?.isInvalidated,
    ).toBe(true);
  });
});
