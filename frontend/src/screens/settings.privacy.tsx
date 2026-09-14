// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { NoticeCasesCard } from "./noticecases";
import { ConsentPurposesCard, PrivacyInboxCard } from "./privacy";
import { ConfirmSubmissionsPanel } from "./privacy.corrections";
import { RestrictedRecordsCard } from "./restrictedrecords";
import { RetentionCard } from "./retention";

/**
 * The governance page's lanes, in the order somebody works them.
 *
 * Extracted from settings.tsx, which is frozen at its length: the privacy page
 * grows a lane roughly every slice, and each one was costing that file two
 * lines it is not allowed to spend. Here they cost nothing, and the ordering
 * argument lives next to the thing it orders.
 */
export function PrivacyLanes() {
  return (
    <>
      <ConsentPurposesCard />
      {/* The retention ladder sits under the purpose catalogue and above the
          DSR inbox: what the installation keeps by default, before the requests
          that override it case by case. */}
      <RetentionCard />
      {/* What the ladder's statutory floor is holding right now, under the
          ladder that explains why: an erasure that met a Handelsbrief
          restricted it rather than destroying it, and the controller has to be
          able to see that without opening the audit trail. */}
      <RestrictedRecordsCard />
      <PrivacyInboxCard />
      {/* Beneath the formal requests, because a correction is the same act
          arriving informally: somebody typed it into the link we mailed them
          rather than filing a rights request, and the officer answering one is
          the officer answering the other. */}
      <ConfirmSubmissionsPanel />
      {/* The disclosure duties last, under the requests somebody sent us: a
          subject-request queue is work that was asked for, and this is the work
          nobody asked for because they do not yet know we hold their data. Same
          grant, same officer, different question. */}
      <NoticeCasesCard />
    </>
  );
}
