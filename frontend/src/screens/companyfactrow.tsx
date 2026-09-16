// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Sparkles, X } from "lucide-react";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useRecordZone } from "../app/recordzone";
import { Badge } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { EvidenceMark } from "../design-system/evidencemark";
import { IconAction } from "../design-system/iconaction";
import { PanelRow } from "../design-system/panel";
import { provenanceLabel } from "../design-system/trust";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";
import { settleFacts } from "./companyfactspanel";
import { derivedSource } from "./evidencesource";
import { EvidenceVerdict, factClaim } from "./evidenceverdict";
import { factFieldLabelKey } from "./factview";
import "./company360.css";

type CompanyFact = components["schemas"]["CompanyFact"];
type FactSuspectReason = NonNullable<CompanyFact["suspect_reason"]>;

const FACT_SUSPECT_LABELS: Record<FactSuspectReason, MessageKey> = {
  phone_shaped_location: "co.factSuspect.phoneShapedLocation",
  not_a_phone: "co.factSuspect.notAPhone",
  not_a_year: "co.factSuspect.notAYear",
  not_an_email: "co.factSuspect.notAnEmail",
  not_a_size: "co.factSuspect.notASize",
};

/**
 * FactRow: one stated or read fact, in the same meta-row shape the contact
 * page reads its memory in (design-system/composed.css `.meta-row`). A meta
 * line names the field and its source, the value sits on its own line under
 * it, and the two verdict verbs plus removal trail at a fixed x. The grid has
 * no face column here: a fact names no counterparty to put one beside, which
 * is what `.meta-row:not(:has(> .avatar))` reads off the row itself rather
 * than being told.
 *
 * `label` is absent when the field above this row already named it: a group
 * with several Locations prints "Location" once and lets its values follow
 * bare, rather than repeating a word the reader has already read.
 */
export function FactRow({
  companyId,
  fact,
  label,
  canEdit,
  onOpenHistory,
}: Readonly<{
  companyId: string;
  fact: CompanyFact;
  label?: string;
  canEdit: boolean;
  onOpenHistory?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const [removing, setRemoving] = useState(false);
  const source = derivedSource(fact, locale, recordZone);
  return (
    <PanelRow className="meta-row">
      <span className="meta-row-line t-caption">
        {label && <span className="t-label">{label}</span>}
        {source && (
          <span className="pe-source">
            {/* Words, not a badge: a pill on every row of a twenty-row card is
                twenty pills, and the memory card this row mirrors names its
                source in plain words with the kind's glyph. The sparkle is the
                agent arm's mark alone, so a connector or a colleague never
                reads as a model's claim. */}
            {source.provenance.kind === "agent" && (
              <Sparkles size={13} aria-hidden="true" />
            )}
            {provenanceLabel(source.provenance, t, undefined)}
          </span>
        )}
      </span>
      <span className="meta-row-entry t-body">
        <EvidenceMark
          value={fact.value}
          source={source}
          onOpenHistory={onOpenHistory}
        />
        {/* The value contradicts its own field: a phone number filed as a
            location, a register number filed as a headcount. The fact is
            still shown with its evidence, since hiding it would be a worse
            answer than flagging it, and the reader is the one who can tell. */}
        {fact.suspect_reason && (
          <span className="co-fact-suspect">
            <Badge tone="warn">
              {t(FACT_SUSPECT_LABELS[fact.suspect_reason])}
            </Badge>
          </span>
        )}
      </span>
      <span className="meta-row-action">
        <EvidenceVerdict
          companyId={companyId}
          claim={factClaim(companyId, fact)}
          canEdit={canEdit}
        />
        {/* Removal is the verb correction cannot spell. A correction says
            "this value is wrong"; this says "this is not a fact about this
            company", which is the honest answer to a customer who left or a
            phone number read off the wrong page. */}
        {canEdit && (
          <IconAction
            label={t("co.facts.remove", {
              value: fact.value,
            })}
            icon={<X aria-hidden />}
            small
            onClick={() => setRemoving(true)}
          />
        )}
      </span>
      {removing && (
        <RemoveFactConfirm
          companyId={companyId}
          fact={fact}
          canEdit={canEdit}
          onClose={() => setRemoving(false)}
        />
      )}
    </PanelRow>
  );
}

function RemoveFactConfirm({
  companyId,
  fact,
  canEdit,
  onClose,
}: Readonly<{
  companyId: string;
  fact: CompanyFact;
  // Re-read at CONFIRM time, not only at open time. A grant can be withdrawn
  // while this dialog stands, and a confirm that fired on the answer from
  // thirty seconds ago is a write the reader is no longer allowed to make.
  canEdit: boolean;
  onClose: () => void;
}>): ReactNode {
  const t = useT();
  const queryClient = useQueryClient();
  const remove = useMutation({
    // The fact travels as a variable rather than through this closure: the
    // click belongs to the committed render, so what it passes cannot be
    // older than the row that carried it.
    mutationFn: async (doomed: CompanyFact) => {
      const { error } = await api.DELETE("/companies/{id}/facts/{factKey}", {
        params: {
          path: {
            id: companyId,
            factKey: `${doomed.field}:${doomed.value_key}`,
          },
          ...ifMatch(requireVersion(doomed.version)),
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      await settleFacts(queryClient, companyId);
      onClose();
    },
  });
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("co.facts.removeTitle")}
      confirmLabel={t("co.facts.removeConfirm")}
      confirmVariant="danger"
      pending={remove.isPending}
      error={remove.error ? problemMessageOf(remove.error, t) : undefined}
      confirmReason={canEdit ? undefined : t("record.notYoursToChange")}
      onConfirm={() => remove.mutate(fact)}
    >
      <p>
        {t("co.facts.removeAsk", {
          field: t(factFieldLabelKey(fact.field)),
          value: fact.value,
        })}
      </p>
    </ConfirmModal>
  );
}
