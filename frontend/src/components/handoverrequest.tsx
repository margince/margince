// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Asking a colleague to hand over the contact or company they own, from the
// access panel. The owner answers on their Worklist.

import {
  type TransferableRecordType,
  useOwnershipRequest,
  useOwnershipRequestActions,
} from "../lib/ownershiprequest";
import {
  type AskedRecord,
  type AskWords,
  OwnerAsk,
  useOwnerToAsk,
} from "./ownerask";

const HANDOVER_WORDS: AskWords = {
  ask: "handover.ask",
  requested: "handover.requested",
  sent: "handover.sent",
  withdraw: "handover.withdraw",
  withdrawn: "handover.withdrawn",
};

/**
 * The ask to be handed a contact or company. Beside the edit access ask it is
 * the second choice, so it draws as a link.
 */
export function HandoverRequest({
  kind,
  record,
}: Readonly<{ kind: TransferableRecordType; record: AskedRecord }>) {
  const ownerId = useOwnerToAsk(kind, record);
  const id = record.id ?? "";
  const open = useOwnershipRequest(kind, id, ownerId !== undefined);
  const actions = useOwnershipRequestActions();
  return (
    <OwnerAsk
      kind={kind}
      id={id}
      ownerId={ownerId}
      open={open}
      actions={actions}
      words={HANDOVER_WORDS}
      variant="link"
    />
  );
}
