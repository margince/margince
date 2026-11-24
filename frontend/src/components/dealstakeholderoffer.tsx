// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Checkbox } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { readEmailPresentation } from "../design-system/emailmessage";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf, unwrap } from "../lib/problem";
import type { Relationship } from "./relationships";
import { fetchRelationships, invalidateAfterEdge } from "./relationships.edges";

type EmailPresentation = components["schemas"]["EmailPresentation"];

/** A person on the mail who could become a stakeholder on the deal. */
export type StakeholderCandidate = Readonly<{
  contactId: string;
  name: string;
}>;

/**
 * The mail's people the offer names: contacts outside the workspace who hold
 * no seat on the deal yet. A party with a seat is a colleague, and a party with
 * no contact the reader may see has nobody to add.
 */
export function stakeholderCandidates(
  mail: EmailPresentation,
  stakeholders: readonly Relationship[],
): StakeholderCandidate[] {
  const seated = new Set(stakeholders.map((seat) => seat.contact_id));
  const found = new Map<string, StakeholderCandidate>();
  for (const party of [...mail.from, ...mail.to, ...mail.cc, ...mail.bcc]) {
    const contactId = party.contact_id;
    if (!contactId || party.user_id || seated.has(contactId)) continue;
    if (!found.has(contactId)) {
      found.set(contactId, {
        contactId,
        name: party.display_name?.trim() || party.address,
      });
    }
  }
  return [...found.values()];
}

/** What runs once the offer is settled; the hook waits for it. */
type OfferDone = () => unknown;

type Offer = Readonly<{
  deal: string;
  people: readonly StakeholderCandidate[];
  done: OfferDone;
}>;

/**
 * After a colleague links a mail to a deal by hand, offers to make the mail's
 * people stakeholders, so their next mail lands on the deal by the stakeholder
 * rule. `offer` resolves once it knows whether to ask; `done` runs when the
 * offer is answered, or at once when nobody is left to offer.
 */
export function useDealStakeholderOffer(): Readonly<{
  offer: (mail: string, deal: string, done: OfferDone) => Promise<void>;
  asking: boolean;
  element: ReactNode;
}> {
  const t = useT();
  const toast = useToast();
  const [open, setOpen] = useState<Offer | null>(null);
  const offer = async (mail: string, deal: string, done: OfferDone) => {
    let people: StakeholderCandidate[];
    try {
      const [presentation, stakeholders] = await Promise.all([
        readEmailPresentation(mail),
        fetchRelationships({ deal_id: deal }),
      ]);
      people = stakeholderCandidates(presentation, stakeholders);
    } catch {
      // The link itself landed. Without the mail's people there is nobody to
      // name, so the reader hears that instead of seeing no offer.
      toast.show(t("rel.stakeholderOffer.unread"), { tone: "warning" });
      people = [];
    }
    if (people.length === 0) {
      await done();
      return;
    }
    setOpen({ deal, people, done });
  };
  const element = open ? (
    <StakeholderOfferDialog
      key={open.deal}
      offer={open}
      onClose={() => {
        setOpen(null);
        void open.done();
      }}
    />
  ) : null;
  return { offer, asking: open !== null, element };
}

function StakeholderOfferDialog({
  offer,
  onClose,
}: Readonly<{ offer: Offer; onClose: () => void }>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [people, setPeople] = useState(offer.people);
  const [chosen, setChosen] = useState<readonly string[]>(() =>
    offer.people.map((person) => person.contactId),
  );
  // Each person is their own write, as the deal page's Add stakeholder makes
  // it. The ones that landed leave the list, so a retry adds only the rest.
  const add = useMutation({
    mutationFn: async (contacts: readonly string[]) => {
      const results = await Promise.allSettled(
        contacts.map(async (contactId) => {
          unwrap(
            await api.POST("/relationships", {
              body: {
                kind: "deal_stakeholder",
                source: "manual",
                deal_id: offer.deal,
                contact_id: contactId,
              },
            }),
          );
          return contactId;
        }),
      );
      const added = new Set(
        results.flatMap((result) =>
          result.status === "fulfilled" ? [result.value] : [],
        ),
      );
      if (added.size > 0) invalidateAfterEdge(queryClient, true);
      setPeople((left) => left.filter((p) => !added.has(p.contactId)));
      setChosen((left) => left.filter((id) => !added.has(id)));
      const refused = results.find((result) => result.status === "rejected");
      if (refused) throw refused.reason;
      return added.size;
    },
    onSuccess: () => {
      toast.show(t("rel.stakeholderOffer.added"));
      onClose();
    },
  });
  return (
    <ConfirmModal
      open
      onClose={onClose}
      intent="form"
      title={t("rel.stakeholderOffer.title")}
      confirmLabel={t("rel.stakeholderOffer.confirm")}
      cancelLabel={t("rel.stakeholderOffer.decline")}
      confirmDisabled={chosen.length === 0}
      onConfirm={() => add.mutate(chosen)}
      pending={add.isPending}
      error={add.isError ? problemMessageOf(add.error, t) : null}
    >
      <p>{t("rel.stakeholderOffer.body")}</p>
      <fieldset className="field-multiselect">
        <legend className="t-name">{t("rel.stakeholderOffer.legend")}</legend>
        {people.map((person) => (
          <Checkbox
            key={person.contactId}
            label={person.name}
            checked={chosen.includes(person.contactId)}
            onChange={(event) =>
              setChosen((now) =>
                event.target.checked
                  ? [...now, person.contactId]
                  : now.filter((id) => id !== person.contactId),
              )
            }
          />
        ))}
      </fieldset>
    </ConfirmModal>
  );
}
