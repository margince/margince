// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type {
  ContactCase,
  ContactCaseMember,
  ContactCaseOutcome,
} from "../components/contactcase.queries";

// Four contacts that may be duplicates. Ana's and Dana's records were marked
// as different people, so a review opened on Ana's ticks the first three only.
export const ANA = "01a05500-0000-7000-8000-00000000c0a1";
export const BEN = "01a05500-0000-7000-8000-00000000c0a2";
export const CLEO = "01a05500-0000-7000-8000-00000000c0a3";
export const DANA = "01a05500-0000-7000-8000-00000000c0a4";

const OWNER_ANA = "01a05500-0000-7000-8000-00000000a0a1";
const OWNER_BEN = "01a05500-0000-7000-8000-00000000a0a2";
const OWNER_CLEO = "01a05500-0000-7000-8000-00000000a0a3";
const OWNER_DAN = "01a05500-0000-7000-8000-00000000a0a4";

function member(
  id: string,
  name: string,
  ownerId: string,
  ownerName: string,
  email: string,
): ContactCaseMember {
  return {
    id,
    name,
    email,
    company_name: "Northwind GmbH",
    owner_id: ownerId,
    owner_name: ownerName,
    version: 3,
    created_at: "2026-09-01T09:00:00Z",
  };
}

export function contactCase(over: Partial<ContactCase> = {}): ContactCase {
  return {
    anchor_id: ANA,
    revision: "rev-1",
    status: "open",
    members: [
      member(ANA, "Mira Kovac", OWNER_ANA, "Ana Owner", "mira@northwind.de"),
      member(
        BEN,
        "Mira Kovač",
        OWNER_BEN,
        "Ben Seller",
        "m.kovac@northwind.de",
      ),
      member(CLEO, "M. Kovac", OWNER_CLEO, "Cleo Reader", "mira.k@gmail.com"),
      member(DANA, "Mira Kovacs", OWNER_DAN, "Dan Gone", "mira@kovacs.hu"),
    ],
    edges: [
      edge("p1", ANA, BEN, "open", 0.92),
      edge("p2", BEN, CLEO, "open", 0.81),
      edge("p3", CLEO, DANA, "open", 0.74),
      edge("p4", ANA, DANA, "not_a_duplicate", 0.7),
    ],
    owners: [
      owner(OWNER_ANA, "Ana Owner", "full", [ANA]),
      owner(OWNER_BEN, "Ben Seller", "full", [BEN]),
      owner(OWNER_CLEO, "Cleo Reader", "read", [CLEO]),
      owner(OWNER_DAN, "Dan Gone", "full", [DANA]),
    ],
    confidence: 0.92,
    too_large: false,
    policy_enabled: false,
    can_merge: true,
    merge_mode: "ordinary",
    reason: null,
    ...over,
  };
}

function edge(
  pairId: string,
  left: string,
  right: string,
  status: "open" | "not_a_duplicate",
  confidence: number,
) {
  return {
    pair_id: pairId,
    left_id: left,
    right_id: right,
    confidence,
    status,
    can_dismiss: true,
    evidence: [
      {
        field: "full_name",
        left_value: "Mira Kovac",
        right_value: "Mira Kovač",
        signal: "collide",
        score: confidence,
      },
    ],
  };
}

function owner(
  userId: string,
  name: string,
  seat: "full" | "read",
  memberIds: string[],
) {
  return { user_id: userId, name, seat, member_ids: memberIds };
}

/**
 * The merge of Ana, Ben and Cleo into Ana, as the preview answers it. Cleo's
 * read seat keeps read access. Ben's deactivated account gets nothing.
 */
export function previewOutcome(
  over: Partial<ContactCaseOutcome> = {},
): ContactCaseOutcome {
  return {
    committed: false,
    survivor_id: ANA,
    merged_ids: [BEN, CLEO],
    owner_id: OWNER_ANA,
    mode: "ordinary",
    shares: [{ user_id: OWNER_CLEO, name: "Cleo Reader", access: "read" }],
    skipped: [{ user_id: OWNER_BEN, name: "Ben Seller", reason: "inactive" }],
    ...over,
  };
}

export const resolvedCase = (): ContactCase =>
  contactCase({
    status: "resolved",
    can_merge: false,
    merge_mode: "none",
    reason: "resolved",
    resolution: {
      survivor_id: BEN,
      resolved_by: OWNER_BEN,
      resolved_by_name: "Ben Seller",
      resolved_at: "2026-10-27T10:00:00Z",
    },
  });
