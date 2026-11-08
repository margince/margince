// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { Receipt } from "./handled";
import { receiptAuditId } from "./receiptreview";

const base: Receipt = {
  id: "11111111-1111-4111-8111-111111111111",
  kind: "deal_close_lost",
  summary: "Closed as lost",
  occurred_at: "2026-11-08T08:00:00Z",
};

describe("receiptAuditId", () => {
  it("takes the audit entry the server names", () => {
    expect(
      receiptAuditId({
        ...base,
        source: "correction",
        audit_log_id: "22222222-2222-4222-8222-222222222222",
      }),
    ).toBe("22222222-2222-4222-8222-222222222222");
  });

  it("never turns an approval's id into an audit entry", () => {
    expect(
      receiptAuditId({
        ...base,
        source: "approval",
        undo: { audit_log_id: base.id, version: 1, reversed: false },
      }),
    ).toBeUndefined();
  });

  it("reads an older server's correction through its undo or review", () => {
    expect(
      receiptAuditId({
        ...base,
        undo: {
          audit_log_id: "33333333-3333-4333-8333-333333333333",
          version: 1,
          reversed: false,
        },
      }),
    ).toBe("33333333-3333-4333-8333-333333333333");
    expect(receiptAuditId(base)).toBeUndefined();
  });
});
