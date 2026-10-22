// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useEntityName } from "../../components/entityref.queries";
import { Checkbox } from "../../design-system/atoms";
import { useT } from "../../i18n";
import type { Approval } from "../../lib/approvals.queries";
import { useCanWrite } from "../../lib/capability";
import { contactQuery } from "../../lib/contact.queries";
import { useMe } from "../../lib/session";

// The two boxes on a LinkedIn match card. One takes the export's position as
// the contact's job title. The other takes the company the import placed the
// connection at as their current employer. The server stages a box ticked only
// when the contact has nothing there. Accepting sends the boxes as the edit.

export type LinkedInChoice = Readonly<{
  use_as_title: boolean;
  use_as_employer: boolean;
}>;

type LinkedInOffer = Readonly<{
  staged: LinkedInChoice;
  // The employer box shows only for a company this reader can see, to a reader
  // who may make the employer write. The server refuses the rest.
  employerName: string | null;
  // Where the contact works today, as this reader may see it.
  employerNow: string | null;
  titleOffered: boolean;
  choice: LinkedInChoice;
  setChoice: (next: LinkedInChoice) => void;
  // The edit to send on accept, or undefined when the boxes are as staged.
  edited: Record<string, unknown> | undefined;
}>;

function text(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}

/**
 * The company the employer box names, or null when the reader may not make
 * the employer write or cannot see the company. Until the grants and the company
 * read have answered, `settled` is false and the staged value stands, so a
 * quick accept does not untick the box. A read that failed is no answer
 * either; only a company the reader cannot see is.
 */
function useEmployerOffer(
  companyId: string | undefined,
  company: ReturnType<typeof useEntityName>,
): { name: string | null; settled: boolean } {
  // The box creates an employment or promotes one the contact holds there. A
  // held one that is not primary is on no read the card makes, so ask both.
  const mayCreate = useCanWrite("relationship", "create");
  const mayPromote = useCanWrite("relationship", "update");
  const mayEmploy = mayCreate && mayPromote;
  const grantsPending = useMe().isPending;
  const known = company.name !== null || company.reading === "unnamed";
  return {
    name: mayEmploy && companyId ? company.name : null,
    settled: !grantsPending && (!mayEmploy || !companyId || known),
  };
}

/**
 * What a `linkedin_match` card offers, and the boxes as the reader set them.
 * Undefined for any other kind, and for a card staged before the boxes
 * existed, which applies only the profile link.
 */
export function useLinkedInChoice(
  approval: Approval,
): LinkedInOffer | undefined {
  const change = approval.proposed_change ?? {};
  const offered =
    approval.kind === "linkedin_match" &&
    typeof change.use_as_title === "boolean" &&
    typeof change.use_as_employer === "boolean";
  const companyId = offered ? text(change.employer_company_id) : undefined;
  const company = useEntityName("company", companyId);
  const employer = useEmployerOffer(companyId, company);
  const contact = useQuery(
    contactQuery(offered ? text(change.contact_id) : undefined),
  );
  // Held per approval, so a row that is handed another card starts from that
  // card's staged boxes rather than from the last reader's ticks.
  const [held, setHeld] = useState<
    { id: string; choice: LinkedInChoice } | undefined
  >(undefined);
  if (!offered) return undefined;
  const staged = {
    use_as_title: change.use_as_title === true,
    use_as_employer: change.use_as_employer === true,
  };
  const titleOffered = text(change.connection_position) !== undefined;
  const choice = held?.id === approval.id ? held.choice : staged;
  const sent = {
    use_as_title: titleOffered && choice.use_as_title,
    use_as_employer: employer.settled
      ? employer.name !== null && choice.use_as_employer
      : staged.use_as_employer,
  };
  const same =
    sent.use_as_title === staged.use_as_title &&
    sent.use_as_employer === staged.use_as_employer;
  return {
    staged,
    employerName: employer.name,
    employerNow: contact.data?.employer?.company_name ?? null,
    titleOffered,
    choice,
    setChoice: (next) => setHeld({ id: approval.id, choice: next }),
    edited: same ? undefined : { ...change, ...sent },
  };
}

export function LinkedInMatchChoice({
  offer,
  decided,
  disabled,
}: Readonly<{
  offer: LinkedInOffer | undefined;
  decided?: boolean;
  disabled?: boolean;
}>) {
  const t = useT();
  if (decided || !offer) return null;
  if (!offer.titleOffered && offer.employerName === null) return null;
  return (
    <div className="linkedin-match-choice">
      {offer.titleOffered && (
        <Checkbox
          checked={offer.choice.use_as_title}
          disabled={disabled}
          onChange={(event) =>
            offer.setChoice({
              ...offer.choice,
              use_as_title: event.currentTarget.checked,
            })
          }
          label={t("approval.linkedin.useAsTitle")}
        />
      )}
      {offer.employerName !== null && (
        <Checkbox
          checked={offer.choice.use_as_employer}
          disabled={disabled}
          onChange={(event) =>
            offer.setChoice({
              ...offer.choice,
              use_as_employer: event.currentTarget.checked,
            })
          }
          label={t("approval.linkedin.useAsEmployer", {
            company: offer.employerName,
          })}
        />
      )}
      {offer.employerName !== null && offer.employerNow !== null && (
        <p className="t-sub">
          {t("approval.linkedin.employerNow", { company: offer.employerNow })}
        </p>
      )}
    </div>
  );
}
