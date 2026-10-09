// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Deciding a tag suggestion where the Worklist shows it: the suggested word,
// the mail and notes that raised it, and Accept or Dismiss. After Accept the
// card makes the tags panel's own offer: a contact's company, or a company's
// contacts.

import { useState } from "react";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Row, Stack } from "../design-system/stack";
import { TagPill } from "../design-system/tagpill";
import { useToast } from "../design-system/toast";
import { ProvenanceTag, StagingCard } from "../design-system/trust";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { ContactsTagOffer, EmployerTagOffer } from "./tagfollow";
import {
  type TagSuggestion,
  useContactEmployer,
  useDecideTagSuggestion,
  useTagSuggestion,
  useTagSuggestionSettled,
} from "./tagsuggestion.queries";
import type { WorklistItem } from "./worklist.queries";

export function TagSuggestionDecision({
  item,
}: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const query = useTagSuggestion(item.id);
  const [accepted, setAccepted] = useState<TagSuggestion | null>(null);
  if (accepted) {
    return <AcceptedFollowUp suggestion={accepted} />;
  }
  if (query.isPending) {
    return null;
  }
  if (query.isError) {
    return <p className="t-caption">{t("tagSuggestion.unavailable")}</p>;
  }
  if (query.data === null) {
    return <p className="t-caption">{t("tagSuggestion.decided")}</p>;
  }
  return <TagSuggestionCard suggestion={query.data} onAccepted={setAccepted} />;
}

function TagSuggestionCard({
  suggestion,
  onAccepted,
}: Readonly<{
  suggestion: TagSuggestion;
  onAccepted: (accepted: TagSuggestion) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const decide = useDecideTagSuggestion();
  const zone = viewerZone();
  return (
    <StagingCard>
      <Stack gap="2">
        <Row gap="2">
          <ProvenanceTag
            provenance={{ kind: "agent", agent: t("tagSuggestion.agent") }}
          />
          <TagPill
            name={suggestion.tag.name}
            tone={suggestion.tag.color ?? undefined}
          />
        </Row>
        <p className="t-caption">{t("tagSuggestion.citedHeading")}</p>
        <ul className="t-caption">
          {suggestion.evidence.map((evidence) => (
            <li key={evidence.activity_id}>
              {t("tagSuggestion.evidence", {
                kind: t(evidenceKindKey(evidence.kind)),
                subject: evidence.subject ?? t("tagSuggestion.noSubject"),
                when: formatDate(evidence.occurred_at, locale, zone),
              })}
            </li>
          ))}
        </ul>
        <ErrorLine error={decide.error} />
        <Row gap="2">
          <Button
            variant="ai"
            pending={decide.isPending && decide.variables?.verb === "accept"}
            disabled={decide.isPending}
            onClick={() =>
              decide.mutate(
                { id: suggestion.id, verb: "accept" },
                { onSuccess: onAccepted },
              )
            }
          >
            {t("tagSuggestion.accept")}
          </Button>
          <Button
            variant="ghost"
            pending={decide.isPending && decide.variables?.verb === "dismiss"}
            disabled={decide.isPending}
            onClick={() =>
              decide.mutate(
                { id: suggestion.id, verb: "dismiss" },
                { onSuccess: () => toast.show(t("tagSuggestion.dismissed")) },
              )
            }
          >
            {t("tagSuggestion.dismiss")}
          </Button>
        </Row>
      </Stack>
    </StagingCard>
  );
}

const EVIDENCE_KINDS = {
  email: "tagSuggestion.kind.email",
  meeting: "tagSuggestion.kind.meeting",
  note: "tagSuggestion.kind.note",
  call: "tagSuggestion.kind.call",
} as const;

// A kind from a newer server reads as a note rather than as its identifier.
function evidenceKindKey(kind: string) {
  return kind in EVIDENCE_KINDS
    ? EVIDENCE_KINDS[kind as keyof typeof EVIDENCE_KINDS]
    : EVIDENCE_KINDS.note;
}

/**
 * After an acceptance: the offer to carry the word to the contact's company or
 * the company's contacts. The Worklist row goes once the reader answers it.
 */
function AcceptedFollowUp({
  suggestion,
}: Readonly<{ suggestion: TagSuggestion }>) {
  const t = useT();
  const settled = useTagSuggestionSettled();
  const contact =
    suggestion.entity_type === "contact" ? suggestion.entity_id : undefined;
  const employer = useContactEmployer(contact);
  const tag = { id: suggestion.tag.tag_id, name: suggestion.tag.name };
  return (
    <Stack gap="2">
      <p className="t-caption">
        {t("tagSuggestion.accepted", {
          tag: suggestion.tag.name,
          record: suggestion.entity_name,
        })}
      </p>
      {suggestion.entity_type === "company" ? (
        <ContactsTagOffer
          companyID={suggestion.entity_id}
          tag={tag}
          onClose={settled}
        />
      ) : (
        employer.data && (
          <EmployerTagOffer
            employer={employer.data}
            tag={tag}
            onClose={settled}
          />
        )
      )}
    </Stack>
  );
}
