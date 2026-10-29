// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ErrorLine } from "../../design-system/errorline";
import { useAddEmployment } from "./addemploymentmodal";
import { useCanEditEmployment, useEmploymentActions } from "./contactemployers";
import type { Contact360 } from "./contactrail";
import {
  EmployerProposal,
  type ProfileField,
  useEmployerProposal,
} from "./employerproposal";

/**
 * The rail's employer row while the contact works nowhere and a signature
 * named a company: the Company card's proposal, through the same verbs.
 */
export function ProposedEmployerRow({
  view,
  fact,
}: Readonly<{ view: Contact360; fact: ProfileField }>) {
  const contactId = view.contact.id;
  const canEdit = useCanEditEmployment(view.contact);
  const actions = useEmploymentActions(contactId);
  const verbs = useEmployerProposal(contactId, actions.invalidate);
  const adding = useAddEmployment(
    contactId,
    actions.create,
    view.employments?.data ?? [],
  );
  return (
    <>
      <EmployerProposal
        fact={fact}
        verbs={verbs}
        canEdit={canEdit}
        onLink={(company) =>
          adding.open({
            term: fact.value,
            company,
            onLinked: () => verbs.confirm(fact),
          })
        }
      />
      <ErrorLine error={verbs.verdict.error} />
      {adding.modal}
    </>
  );
}
