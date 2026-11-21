// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Asking the owner of a record for edit access. It sits beside the sentence
// that says the record is not the reader's to change. The owner stays the
// owner; the reader gets a write share once the owner approves.

import {
  type EditAccessRecordType,
  useEditAccessRequest,
  useEditAccessRequestActions,
} from "./editaccessapi";
import {
  type AskedRecord,
  type AskWords,
  OwnerAsk,
  useOwnerToAsk,
} from "./ownerask";

const EDIT_ACCESS_WORDS: AskWords = {
  ask: "editAccess.ask",
  requested: "editAccess.requested",
  sent: "editAccess.sent",
  withdraw: "editAccess.withdraw",
  withdrawn: "editAccess.withdrawn",
};

/** The ask for edit access to a record a colleague owns. */
export function EditAccessRequest({
  kind,
  record,
}: Readonly<{ kind: EditAccessRecordType; record: AskedRecord }>) {
  const ownerId = useOwnerToAsk(kind, record);
  const id = record.id ?? "";
  const open = useEditAccessRequest(kind, id, ownerId !== undefined);
  const actions = useEditAccessRequestActions();
  return (
    <OwnerAsk
      kind={kind}
      id={id}
      ownerId={ownerId}
      open={open}
      actions={actions}
      words={EDIT_ACCESS_WORDS}
      variant="primary"
    />
  );
}
