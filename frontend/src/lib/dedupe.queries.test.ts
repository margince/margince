// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient } from "@tanstack/react-query";
import { expect, it } from "vitest";
import { invalidateAfterContactMerge } from "./dedupe.queries";
import { contactCasesKey, dsrsKey } from "./workkeys";

const RETIRED = "01a05500-0000-7000-8000-00000000c0a1";

// A request opened on the retired contact names it by label, so the open
// drawer reads the queue again once the merge lands.
it("stales the subject-request queue and the duplicate cases after a contact merge", async () => {
  const client = new QueryClient();
  client.setQueryData([...dsrsKey, "open"], { data: [] });
  client.setQueryData([...contactCasesKey, RETIRED], null);

  await invalidateAfterContactMerge(client, [RETIRED]);

  expect(client.getQueryState([...dsrsKey, "open"])?.isInvalidated).toBe(true);
  expect(
    client.getQueryState([...contactCasesKey, RETIRED])?.isInvalidated,
  ).toBe(true);
});
