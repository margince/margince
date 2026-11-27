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

  await invalidateAfterContactMerge(client, [RETIRED], undefined);

  expect(client.getQueryState([...dsrsKey, "open"])?.isInvalidated).toBe(true);
  expect(
    client.getQueryState([...contactCasesKey, RETIRED])?.isInvalidated,
  ).toBe(true);
});

// The merged-away contact's 360 is refused once the merge lands; the
// survivor's is the page the reader is sent to.
it("stales the survivor's 360 and leaves the merged-away one unasked", async () => {
  const SURVIVOR = "01a05500-0000-7000-8000-00000000c0a2";
  const client = new QueryClient();
  client.setQueryData(["contact360", RETIRED], {});
  client.setQueryData(["contact360", SURVIVOR], {});

  await invalidateAfterContactMerge(client, [RETIRED, SURVIVOR], SURVIVOR);

  expect(client.getQueryState(["contact360", SURVIVOR])?.isInvalidated).toBe(
    true,
  );
  expect(client.getQueryState(["contact360", RETIRED])?.isInvalidated).toBe(
    false,
  );
});
