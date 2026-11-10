// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { company360 } from "../../testing/company.fixtures";
import { contactsByAddress } from "./composefiling";

type Company360Contact = components["schemas"]["Company360Contact"];

function member(
  contactId: string,
  email: string,
  attachable: boolean,
): Company360Contact {
  return {
    contact_id: contactId,
    full_name: contactId,
    attachable,
    primary_email: email,
    strength: {
      score: 0,
      bucket: "none",
      factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
    },
    deal_roles: [],
    consent: {},
  };
}

function roster(...members: Company360Contact[]) {
  return {
    ...company360,
    contacts: { data: members, page: { has_more: false } },
  };
}

describe("the contact an address belongs to", () => {
  it("prefers the owner the sender may attach to over an earlier one", () => {
    const ownerOf = contactsByAddress(
      undefined,
      roster(
        member("read-share", "buyer@acme.test", false),
        member("own", "Buyer@Acme.test", true),
      ),
    );

    expect(ownerOf(" buyer@acme.test ")).toBe("own");
  });

  it("keeps the first owner when no other is attachable, and the first attachable one", () => {
    const readOnly = contactsByAddress(
      undefined,
      roster(
        member("first", "buyer@acme.test", false),
        member("second", "buyer@acme.test", false),
      ),
    );
    const bothOwn = contactsByAddress(
      undefined,
      roster(
        member("first", "buyer@acme.test", true),
        member("second", "buyer@acme.test", true),
      ),
    );

    expect(readOnly("buyer@acme.test")).toBe("first");
    expect(bothOwn("buyer@acme.test")).toBe("first");
    expect(readOnly("nobody@acme.test")).toBeUndefined();
  });
});
