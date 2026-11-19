// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { MailDraft } from "./composesaveddraftcalls";
import { fieldsOfDraft } from "./composesaveddraftfields";

const DRAFT: MailDraft = {
  id: "md-1",
  anchor_type: "contact",
  anchor_id: "p-1",
  to: [],
  cc: [],
  bcc: [],
  subject: "Pricing for 40 seats",
  body: "Half written",
  version: 1,
  agent_drafted: false,
  created_at: "2026-09-20T09:00:00Z",
  updated_at: "2026-09-20T09:05:00Z",
};

describe("a saved draft read back", () => {
  it("holds each recipient once, where it first stands in To, Cc, Bcc", () => {
    const fields = fieldsOfDraft({
      ...DRAFT,
      to: ["alice@demo.test", "ALICE@DEMO.TEST", "bob@demo.test"],
      cc: ["Bob@Demo.test", "carol@demo.test"],
      bcc: ["alice@demo.test", "carol@demo.test", "dan@demo.test"],
    });
    expect(fields.to).toEqual(["alice@demo.test", "bob@demo.test"]);
    expect(fields.cc).toEqual(["carol@demo.test"]);
    expect(fields.bcc).toEqual(["dan@demo.test"]);
  });
});
