// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { useId } from "react";
import { api } from "../../api/client";
import { useIdempotencyKey } from "../../api/idempotencykey";
import type { components } from "../../api/schema";
import { useEntityName } from "../../components/entityref.queries";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { PanelBody } from "../../design-system/panel";
import type { RecordPickerCandidate } from "../../design-system/recordpicker";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { useGrantRefused } from "../../lib/capability";
import { ProblemError, problemExistingId, unwrap } from "../../lib/problem";
import { sourcePhrase } from "./contactcorrections";
import type { Employment } from "./contactemployers";
import { withheldSections } from "./contactrail";
import { stillHeld } from "./employmentcurrency";
import {
  type ProfileFieldVerdict,
  recordProfileFieldVerdict,
  shownValueOf,
} from "./profilefieldverdict";

type Contact360 = components["schemas"]["Contact360"];
export type ProfileField = components["schemas"]["ContactProfileField"];

/**
 * The company a machine read for this contact that nobody has ruled on yet,
 * offered only while the contact visibly holds no current employment.
 */
export function proposedEmployer(
  view: Contact360,
  employments: readonly Employment[],
): ProfileField | undefined {
  if (withheldSections(view).employments || employments.some(stillHeld)) {
    return undefined;
  }
  return (view.profile_fields ?? []).find(
    (field) =>
      field.field === "company_name" &&
      field.verdict === undefined &&
      field.value.trim() !== "",
  );
}

// The company that already holds the contact's work domain, when the server
// refused to create a second one for it.
function duplicateCompanyId(error: unknown): string | null {
  if (!(error instanceof ProblemError)) return null;
  const existing = problemExistingId(error.problem);
  return existing?.code === "duplicate_domain" ? existing.id : null;
}

export function useEmployerProposal(
  contactId: string,
  invalidate: () => Promise<void>,
) {
  const t = useT();
  const toast = useToast();
  const { key, renew } = useIdempotencyKey();
  const verdict = useMutation({
    mutationFn: ({
      field,
      ruling,
    }: {
      field: ProfileField;
      ruling: ProfileFieldVerdict;
    }) =>
      recordProfileFieldVerdict({
        contactId,
        field,
        shown: shownValueOf(field),
        verdict: ruling,
      }),
    onSuccess: invalidate,
  });
  const createAndLink = useMutation({
    mutationFn: async ({
      field,
      idempotencyKey,
    }: {
      field: ProfileField;
      idempotencyKey: string;
    }) =>
      unwrap(
        await api.POST("/contacts/{id}/employer", {
          params: {
            path: { id: contactId },
            header: { "Idempotency-Key": idempotencyKey },
          },
          body: { company_name: field.value },
        }),
        t,
      ),
    onSuccess: async (linked, { field }) => {
      renew();
      toast.show(
        linked.company_domain
          ? t("contact.proposal.createdWithDomain", {
              company: field.value,
              domain: linked.company_domain,
            })
          : t("contact.proposal.created", { company: field.value }),
      );
      verdict.mutate({ field, ruling: "confirmed" });
      await invalidate();
    },
  });
  return {
    verdict,
    createAndLink,
    confirm: (field: ProfileField) =>
      verdict.mutate({ field, ruling: "confirmed" }),
    dismiss: (field: ProfileField) =>
      verdict.mutate({ field, ruling: "suppressed" }),
    create: (field: ProfileField) =>
      createAndLink.mutate({ field, idempotencyKey: key }),
  };
}

export type EmployerProposalVerbs = ReturnType<typeof useEmployerProposal>;

/**
 * The company a signature named, offered on a contact who works nowhere yet.
 * A reader who may not write the contact sees the fact and no verbs.
 */
export function EmployerProposal({
  fact,
  verbs,
  canEdit,
  onLink,
}: Readonly<{
  fact: ProfileField;
  verbs: EmployerProposalVerbs;
  canEdit: boolean;
  // Opens the employment picker, on the given company or searching the fact.
  onLink: (company?: RecordPickerCandidate) => void;
}>) {
  const t = useT();
  const linkReasonId = useId();
  const linkRefused = useGrantRefused({
    object: "relationship",
    action: "create",
  });
  const createRefused = useGrantRefused({
    object: "company",
    action: "create",
  });
  const duplicateId = duplicateCompanyId(verbs.createAndLink.error);
  const duplicate = useEntityName("company", duplicateId);
  const busy = verbs.createAndLink.isPending || verbs.verdict.isPending;
  return (
    <PanelBody>
      <Callout
        tone="ai"
        kind="standing"
        title={t("contact.proposal.title", {
          source: sourcePhrase(fact.source, t),
          company: fact.value,
        })}
        actions={
          canEdit && (
            <>
              <Button
                variant="primary"
                reasonId={linkRefused ? linkReasonId : undefined}
                disabled={busy}
                onClick={() => onLink()}
              >
                {t("contact.proposal.link")}
              </Button>
              <Button
                reasonId={linkRefused ? linkReasonId : undefined}
                reason={
                  !linkRefused && createRefused
                    ? t("contact.proposal.createDenied")
                    : undefined
                }
                pending={verbs.createAndLink.isPending}
                disabled={verbs.verdict.isPending}
                onClick={() => verbs.create(fact)}
              >
                {t("contact.proposal.create")}
              </Button>
              <Button
                pending={verbs.verdict.isPending}
                disabled={verbs.createAndLink.isPending}
                onClick={() => verbs.dismiss(fact)}
              >
                {t("contact.proposal.dismiss")}
              </Button>
            </>
          )
        }
      >
        {canEdit && t("contact.proposal.ask")}
      </Callout>
      {canEdit && linkRefused && (
        <p className="t-caption" id={linkReasonId}>
          {t("contact.proposal.linkDenied")}
        </p>
      )}
      {duplicateId === null ? (
        <ErrorLine error={verbs.createAndLink.error} />
      ) : (
        <ErrorLine
          actions={
            <Button
              variant="link"
              onClick={() =>
                onLink(
                  duplicate.name === null
                    ? undefined
                    : { id: duplicateId, name: duplicate.name },
                )
              }
            >
              {t("contact.proposal.linkExisting")}
            </Button>
          }
        >
          {duplicate.name
            ? t("contact.proposal.duplicate", { company: duplicate.name })
            : t("contact.proposal.duplicateUnnamed")}
        </ErrorLine>
      )}
    </PanelBody>
  );
}
