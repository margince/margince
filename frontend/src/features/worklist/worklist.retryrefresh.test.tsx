/** @vitest-environment happy-dom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createQueryClient } from "../../app/queryclient";
import { LocaleProvider } from "../../i18n";
import { approvalQueryKey } from "../../lib/approvalread";
import { useApprovalRetry } from "./worklist.acknowledge";

// Retrying a failed approval changes the approval, so the surface reading that
// one approval has to hear about it.
//
// Its key is `["approval", id]`, which no prefix of `["approvals"]` reaches.
//
// A retry that invalidated the plural key alone left the staged approval on
// screen as it was before the retry.
describe("retrying a failed approval", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("refreshes the read of the approval it retried", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response("{}", {
            status: 200,
            headers: { "content-type": "application/json" },
          }),
      ),
    );
    const client = createQueryClient();
    const approval = "ap-1";

    // A resolved entry at the approval's own key. Invalidation marks it stale,
    // which is what a surface reading it would act on.
    client.setQueryData(approvalQueryKey(approval), { id: approval });

    const wrapper = ({ children }: Readonly<{ children: ReactNode }>) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">{children}</LocaleProvider>
      </QueryClientProvider>
    );
    const { result } = renderHook(() => useApprovalRetry(), { wrapper });

    result.current.mutate(approval);

    await waitFor(() =>
      expect(
        client.getQueryState(approvalQueryKey(approval))?.isInvalidated,
      ).toBe(true),
    );
  });
});
