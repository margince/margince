// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Eyebrow } from "../design-system/eyebrow";
import { formatPercent } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { mayDecidePairs, PairActions } from "./contactcase.pair";
import type { ContactCase, ContactCaseOutcome } from "./contactcase.queries";

type Evidence = components["schemas"]["DedupeEvidence"];

// The names contacts.DedupeEvidenceFields can write; any other reads as a
// detail, never as the raw field name.
const FIELD_LABELS: Readonly<Record<string, MessageKey>> = {
  full_name: "contactCase.field.name",
  display_name: "contactCase.field.name",
  legal_name: "contactCase.field.legalName",
  email: "contactCase.field.email",
  phone: "contactCase.field.phone",
  matched_lane: "contactCase.field.matchedOn",
  channel_identity: "contactCase.field.chatAccount",
};

function nameOf(contactCase: ContactCase, id: string): string {
  return contactCase.members.find((member) => member.id === id)?.name ?? "";
}

function EvidenceLine({ evidence }: Readonly<{ evidence: Evidence }>) {
  const t = useT();
  const values = [evidence.left_value, evidence.right_value].filter(Boolean);
  return (
    <li className="t-caption">
      {t(FIELD_LABELS[evidence.field] ?? "contactCase.field.other")}:{" "}
      {values.join(" · ")}
    </li>
  );
}

/** Each pair in the case, with what made it look like one person. */
export function CaseEvidence({
  contactCase,
  onStale,
}: Readonly<{ contactCase: ContactCase; onStale: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const decides = mayDecidePairs(contactCase);
  return (
    <section className="contactcase-section">
      <Eyebrow as="h3">{t("contactCase.evidence.title")}</Eyebrow>
      <ul className="contactcase-list">
        {contactCase.edges.map((edge) => (
          <li key={edge.pair_id} className="contactcase-edge">
            <span>
              {t("contactCase.evidence.pair", {
                left: nameOf(contactCase, edge.left_id),
                right: nameOf(contactCase, edge.right_id),
              })}{" "}
              {edge.status === "not_a_duplicate" ? (
                <Badge tone="warning">
                  {t("contactCase.evidence.different")}
                </Badge>
              ) : (
                <span className="t-caption">
                  {t("contactCase.evidence.match", {
                    percent: formatPercent(edge.confidence, locale),
                  })}
                </span>
              )}
            </span>
            {edge.evidence.length > 0 && (
              <ul className="contactcase-list">
                {edge.evidence.map((evidence) => (
                  <EvidenceLine
                    key={`${evidence.field}-${evidence.signal}`}
                    evidence={evidence}
                  />
                ))}
              </ul>
            )}
            {decides && (
              <PairActions
                contactCase={contactCase}
                edge={edge}
                onStale={onStale}
              />
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

const SKIP_REASONS = {
  inactive: "contactCase.preview.skipped.inactive",
  already_owner: "contactCase.preview.skipped.alreadyOwner",
  already_granted: "contactCase.preview.skipped.alreadyGranted",
} as const satisfies Record<
  components["schemas"]["ContactCaseSkippedOwner"]["reason"],
  MessageKey
>;

/** What the merge will do, as the server worked it out without writing. */
export function CasePreview({
  contactCase,
  outcome,
}: Readonly<{ contactCase: ContactCase; outcome: ContactCaseOutcome }>) {
  const t = useT();
  const ownerName =
    contactCase.owners.find((owner) => owner.user_id === outcome.owner_id)
      ?.name ?? "";
  return (
    <section className="contactcase-section" data-testid="contact-case-preview">
      <Eyebrow as="h3">{t("contactCase.preview.title")}</Eyebrow>
      <p>
        {t("contactCase.preview.keeps", {
          name: nameOf(contactCase, outcome.survivor_id),
          owner: ownerName,
        })}
      </p>
      <PreviewList
        title={t("contactCase.preview.mergedTitle")}
        lines={outcome.merged_ids.map((id) => nameOf(contactCase, id))}
      />
      <PreviewList
        title={t("contactCase.preview.sharesTitle")}
        empty={t("contactCase.preview.noShares")}
        lines={outcome.shares.map((share) =>
          t(
            share.access === "write"
              ? "contactCase.preview.shareWrite"
              : "contactCase.preview.shareRead",
            { name: share.name },
          ),
        )}
      />
      {outcome.skipped.length > 0 && (
        <PreviewList
          title={t("contactCase.preview.skippedTitle")}
          lines={outcome.skipped.map((skip) =>
            t(SKIP_REASONS[skip.reason], { name: skip.name }),
          )}
        />
      )}
      <Callout tone="warning" title={t("contactCase.preview.final")} />
    </section>
  );
}

function PreviewList({
  title,
  lines,
  empty,
}: Readonly<{ title: string; lines: readonly string[]; empty?: string }>) {
  return (
    <div>
      <Eyebrow as="h4">{title}</Eyebrow>
      {lines.length === 0 ? (
        <p className="t-caption">{empty}</p>
      ) : (
        <ul className="contactcase-list">
          {lines.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      )}
    </div>
  );
}
