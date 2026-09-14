// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { day, renderWorklist, stub } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it("lets the scope dial leave the unassigned address", async () => {
  window.location.hash = "#/worklist/unassigned";
  stub(
    day({ scope: "unassigned", scope_options: ["mine", "team", "unassigned"] }),
  );
  const fallback = globalThis.fetch;
  const scopes: string[] = [];
  vi.stubGlobal(
    "fetch",
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(
        input instanceof Request ? input.url : String(input),
        "https://test.local",
      );
      if (url.pathname.endsWith("/worklist"))
        scopes.push(url.searchParams.get("scope") ?? "");
      return fallback(input, init);
    },
  );
  renderWorklist("en", "unassigned");
  await waitFor(() => expect(scopes).toContain("unassigned"));
  await userEvent.click(
    await screen.findByRole("button", { name: en["worklist.scope.mine"] }),
  );
  await waitFor(() => expect(scopes).toContain("mine"));
  await userEvent.click(
    screen.getByRole("button", { name: en["worklist.scope.team"] }),
  );
  await waitFor(() => expect(scopes).toContain("team"));
});
