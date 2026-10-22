// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { createQueryClient } from "../app/queryclient";
import { useContact360 } from "../features/contacts/contact360";
import {
  useReassignTask,
  useTakeOwnership,
} from "../features/worklist/worklist.writes";
import { LocaleProvider } from "../i18n";
import { jsonResponse } from "./worklist.testkit";

vi.mock("../format/now", () => ({
  useNow: () => Date.parse("2026-08-25T12:00:00Z"),
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it.each(["reassign", "take", "conflict", "take-conflict"] as const)(
  "%s refreshes personal attribution and every affected cached view",
  async (action) => {
    const conflict = action === "conflict" || action === "take-conflict";
    let assignee = action === "take" ? "u-colleague" : "u-me";
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const request = input instanceof Request ? input : new Request(input);
        const path = new URL(request.url).pathname;
        if (request.method === "GET" && path.endsWith("/activities/a-1"))
          return jsonResponse({ id: "a-1", version: 1 });
        if (request.method === "PATCH" && path.endsWith("/activities/a-1")) {
          expect(request.headers.get("If-Match")).toBe("1");
          assignee = conflict
            ? "u-colleague"
            : (await request.json()).assignee_id;
          if (conflict)
            return new Response(
              JSON.stringify({
                type: "about:blank",
                status: 409,
                title: "Changed elsewhere",
                code: "version_skew",
              }),
              {
                status: 409,
                headers: { "Content-Type": "application/problem+json" },
              },
            );
          return jsonResponse({
            id: "a-1",
            version: 2,
            links: [
              { entity_type: "lead", entity_id: "l-1" },
              { entity_type: "deal", entity_id: "d-1" },
              { entity_type: "project", entity_id: "project-1" },
            ],
          });
        }
        if (path.endsWith("/me")) return jsonResponse({ user: { id: "u-me" } });
        if (path.endsWith("/contacts/p-1/360"))
          return jsonResponse({
            as_of: "2026-08-25T12:00:00Z",
            contact: { id: "p-1", full_name: "Dana Buyer" },
            sections_omitted: [],
            claims: [],
            next_steps: {
              data: [
                {
                  id: "a-1",
                  subject: "Send the pilot quote",
                  assignee_id: assignee,
                  is_done: false,
                },
              ],
            },
          });
        throw new Error(`Unexpected ${request.method} ${path}`);
      }),
    );
    const client = createQueryClient();
    const affected = [
      ["worklist", "mine", "all", ""],
      ["worklist", "mine", "all", "u-colleague"],
      ["worklist", "team", "all", ""],
      ["company360", "company-1"],
      ["lead", "l-1"],
      ["leads", "open"],
      ["deal", "d-1"],
      ["project", "project-1"],
      ["tasks"],
    ];
    for (const key of affected) client.setQueryData(key, {});
    client.setQueryData(["settings", "profile"], {});
    function Wrapper({ children }: Readonly<{ children: ReactNode }>) {
      return (
        <QueryClientProvider client={client}>
          <LocaleProvider>{children}</LocaleProvider>
        </QueryClientProvider>
      );
    }
    const view = renderHook(() => useContact360("p-1"), { wrapper: Wrapper });
    const assigneeShown = () =>
      view.result.current.data?.next_steps?.data[0]?.assignee_id;
    const mutation = renderHook(
      () => ({ reassign: useReassignTask(), take: useTakeOwnership() }),
      { wrapper: Wrapper },
    );
    await waitFor(() =>
      expect(assigneeShown()).toBe(action === "take" ? "u-colleague" : "u-me"),
    );
    await act(async () => {
      if (action === "take-conflict") {
        await expect(
          mutation.result.current.take.mutateAsync({
            subject: { type: "activity", id: "a-1", label: "Pilot quote" },
            userId: "u-me",
          }),
        ).rejects.toBeDefined();
      } else if (action === "conflict") {
        await expect(
          mutation.result.current.reassign.mutateAsync({
            activityId: "a-1",
            version: 1,
            assigneeId: "u-colleague",
          }),
        ).rejects.toBeDefined();
      } else if (action === "reassign") {
        await mutation.result.current.reassign.mutateAsync({
          activityId: "a-1",
          version: 1,
          assigneeId: "u-colleague",
        });
      } else {
        await mutation.result.current.take.mutateAsync({
          subject: { type: "activity", id: "a-1", label: "Pilot quote" },
          userId: "u-me",
        });
      }
    });
    await waitFor(() =>
      expect(assigneeShown()).toBe(action === "take" ? "u-me" : "u-colleague"),
    );
    for (const key of affected)
      expect(client.getQueryState(key)?.isInvalidated).toBe(true);
    expect(client.getQueryState(["settings", "profile"])?.isInvalidated).toBe(
      false,
    );
    client.clear();
  },
);
