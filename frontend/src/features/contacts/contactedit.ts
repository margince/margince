// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { ADDRESS_FIELDS, addressFrom } from "../../components/companyform";
import {
  contactEditFields,
  mapContactUpdate,
} from "../../components/contactformfields";
import type { RailEdit } from "../../components/railfield";
import { saveRecordEdit } from "../../components/recordedit";
import { rawRecord } from "../../components/recordfieldtypes";
import { useRecordOwners } from "../../components/recordowners";
import { useT } from "../../i18n";
import { useCanWriteRecord } from "../../lib/capability";

type Contact = components["schemas"]["Contact"];

/**
 * How a contact's fields change: the fields, the record they open on and the
 * save. The Research tab's details and the left column both edit through it.
 */
export function useContactEdit(contact: Contact): RailEdit {
  const t = useT();
  const canEdit = useCanWriteRecord("contact", contact) && !contact.archived_at;
  const owners = useRecordOwners(contact.owner_id);
  const linkedin =
    typeof contact.social?.linkedin === "string" ? contact.social.linkedin : "";
  return {
    kind: "contact",
    canEdit,
    fields: [
      ...contactEditFields(t),
      ...ADDRESS_FIELDS,
      {
        // Still reads visibility, and does not offer it. An owner-private
        // contact must name an owner, and this field states the rule the
        // header's toggle obeys.
        key: "owner_id",
        required: contact.visibility === "owner",
        label: "list.owner",
        type: "select",
        options: owners.options,
        hint: owners.hint,
      },
    ],
    record: {
      ...contact,
      original: contact,
      ...addressFrom(contact.address),
      "social.linkedin": linkedin,
    },
    resolveExisting: (_code, id) => ({ screen: "contacts", id }),
    save: async (values, rows, opened, extra) =>
      saveRecordEdit("contact", rawRecord(opened), {
        ...mapContactUpdate(values, rows, opened),
        ...extra,
      }),
  };
}
