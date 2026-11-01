// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { Button, Field } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { DrawerBody, DrawerFoot } from "../design-system/drawerbands";
import { ErrorLine } from "../design-system/errorline";
import { Checkbox, Radio } from "../design-system/field";
import { Select } from "../design-system/select";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemCodeOf, problemStatus } from "../lib/problem";
import { CaseEvidence, CasePreview } from "./contactcase.evidence";
import {
  type ContactCase,
  type ContactCaseDecision,
  type ContactCaseMember,
  type ContactCaseOutcome,
  useContactCaseMerge,
  useContactCasePreview,
} from "./contactcase.queries";
import {
  defaultOwner,
  defaultSelection,
  dismissedWith,
  isConnected,
  memberLabel,
  ownerChoices,
  pairKeep,
  pairLocked,
  pairProblem,
} from "./contactcase.selection";

type Props = Readonly<{
  contactCase: ContactCase;
  startId: string;
  /** The case changed under an earlier preview or merge and was read again. */
  stale: boolean;
  onClose: () => void;
  onStale: () => void;
  onMerged: (outcome: ContactCaseOutcome) => void;
  /** This reviewer merged one pair of a too-large case here. */
  onPairMerged: () => void;
}>;

function selectionProblem(
  contactCase: ContactCase,
  selected: readonly string[],
  owner: string,
): MessageKey | null {
  if (selected.length < 2) return "contactCase.tooFew";
  const pair = pairProblem(contactCase, selected);
  if (pair) return pair;
  if (!isConnected(contactCase, selected)) return "contactCase.notConnected";
  if (!owner) return "contactCase.noOwner";
  return null;
}

/** Why this reader may not merge, when they may not. */
export function notAllowedKey(contactCase: ContactCase): MessageKey | null {
  if (contactCase.can_merge) return null;
  if (contactCase.too_large || contactCase.reason === "too_large") {
    return "contactCase.notAllowed.tooLarge";
  }
  return contactCase.policy_enabled
    ? "contactCase.notAllowed.ownersOnly"
    : "contactCase.notAllowed.noAuthority";
}

function MemberDetail({ member }: Readonly<{ member: ContactCaseMember }>) {
  const t = useT();
  const facts = [member.email, member.company_name, member.title].filter(
    Boolean,
  );
  return (
    <span className="contactcase-member-name">
      <strong>{member.name}</strong>
      {facts.length > 0 && (
        <span className="t-caption">{facts.join(" · ")}</span>
      )}
      <span className="t-caption">
        {member.owner_name
          ? t("contactCase.member.owner", { name: member.owner_name })
          : t("contactCase.member.unowned")}
      </span>
    </span>
  );
}

/** The decision itself: which contacts are duplicates, what stays, who owns it. */
export function CaseForm({
  contactCase,
  startId,
  stale,
  onClose,
  onStale,
  onMerged,
  onPairMerged,
}: Props) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const radioName = useId();
  const [selected, setSelected] = useState(() =>
    defaultSelection(contactCase, startId),
  );
  const [survivor, setSurvivor] = useState(
    () => pairKeep(contactCase, selected) ?? selected[0] ?? "",
  );
  const [owner, setOwner] = useState(() =>
    defaultOwner(contactCase, selected, survivor),
  );
  // Until the reviewer picks an owner, the owner follows the kept record.
  const [ownerPicked, setOwnerPicked] = useState(false);
  const preview = useContactCasePreview();
  const merge = useContactCaseMerge();
  const locked = !contactCase.can_merge;
  const choices = ownerChoices(contactCase, selected);
  const mustKeep = pairKeep(contactCase, selected);
  const problem = selectionProblem(contactCase, selected, owner);
  const body: ContactCaseDecision = {
    revision: contactCase.revision,
    member_ids: [...selected],
    survivor_id: survivor,
    owner_id: owner,
  };
  const previewed =
    preview.data && preview.variables?.body
      ? JSON.stringify(preview.variables.body) === JSON.stringify(body)
      : false;

  const choose = (next: readonly string[], keep: string) => {
    const kept =
      pairKeep(contactCase, next) ??
      (next.includes(keep) ? keep : (next[0] ?? ""));
    setSelected(next);
    setSurvivor(kept);
    const stillEligible = ownerChoices(contactCase, next).some(
      (one) => one.user_id === owner,
    );
    if (!ownerPicked || !stillEligible) {
      setOwner(defaultOwner(contactCase, next, kept));
    }
  };
  const toggle = (id: string, on: boolean) =>
    choose(
      on ? [...selected, id] : selected.filter((one) => one !== id),
      survivor,
    );
  const staleOr = (error: Error) => {
    if (problemCodeOf(error) === "case_changed") onStale();
  };
  const failure = merge.error ?? preview.error;
  const refused =
    failure !== null &&
    (problemStatus(failure) === 403 ||
      problemCodeOf(failure) === "permission_denied");
  const notAllowed = notAllowedKey(contactCase);

  return (
    <>
      <DrawerBody className="form-stack">
        {stale && <Callout tone="warning" title={t("contactCase.stale")} />}
        <p>{t("contactCase.intro")}</p>
        {notAllowed && <Callout tone="info" title={t(notAllowed)} />}
        <ul className="contactcase-list contactcase-members">
          {contactCase.members.map((member) => {
            const ticked = selected.includes(member.id);
            const against = dismissedWith(contactCase, member.id).filter((id) =>
              selected.includes(id),
            );
            const names = contactCase.members
              .filter((one) => against.includes(one.id))
              .map((one) => one.name)
              .join(", ");
            return (
              <li key={member.id} className="contactcase-member">
                <Checkbox
                  label={<MemberDetail member={member} />}
                  checked={ticked}
                  disabled={
                    locked ||
                    (!ticked && against.length > 0) ||
                    pairLocked(contactCase, selected, member.id)
                  }
                  onChange={(event) => toggle(member.id, event.target.checked)}
                />
                <Radio
                  name={radioName}
                  label={t("contactCase.member.keep")}
                  aria-label={t("contactCase.member.keepNamed", {
                    name: memberLabel(contactCase, member.id),
                  })}
                  checked={survivor === member.id}
                  disabled={
                    locked ||
                    !ticked ||
                    (mustKeep !== undefined && mustKeep !== member.id)
                  }
                  onChange={() => choose(selected, member.id)}
                />
                {!ticked && against.length > 0 && (
                  <p className="t-caption">
                    {t("contactCase.member.dismissedWith", { names })}
                  </p>
                )}
              </li>
            );
          })}
        </ul>
        <Field
          label={t("contactCase.ownerLabel")}
          hint={t("contactCase.ownerHint")}
        >
          {(control) => (
            <Select
              id={control.id}
              aria-describedby={control["aria-describedby"]}
              options={choices.map((one) => ({
                value: one.user_id,
                label: one.name,
              }))}
              value={owner}
              placeholder={t("contactCase.ownerPlaceholder")}
              disabled={locked}
              onChange={(next) => {
                setOwnerPicked(true);
                setOwner(next);
              }}
            />
          )}
        </Field>
        {!locked && problem && <p className="t-caption">{t(problem)}</p>}
        <CaseEvidence
          contactCase={contactCase}
          onStale={onStale}
          onPairMerged={onPairMerged}
        />
        {previewed && preview.data && (
          <CasePreview contactCase={contactCase} outcome={preview.data} />
        )}
        {refused ? (
          <Callout tone="danger" title={t("contactCase.refused")} />
        ) : (
          problemCodeOf(failure) !== "case_changed" && (
            <ErrorLine error={failure} />
          )
        )}
      </DrawerBody>
      <DrawerFoot className="actions">
        <Button onClick={onClose} disabled={merge.isPending}>
          {t("create.cancel")}
        </Button>
        {!locked && !previewed && (
          <Button
            variant="primary"
            disabled={problem !== null}
            pending={preview.isPending}
            onClick={() =>
              preview.mutate(
                { anchorId: contactCase.anchor_id, body },
                { onError: staleOr },
              )
            }
          >
            {t("contactCase.previewVerb")}
          </Button>
        )}
        {!locked && previewed && (
          <Button
            variant="danger"
            pending={merge.isPending}
            data-testid="contact-case-merge"
            onClick={() =>
              merge.mutate(
                { anchorId: contactCase.anchor_id, body },
                { onError: staleOr, onSuccess: onMerged },
              )
            }
          >
            {plural("contactCase.merge", selected.length, {
              count: formatNumber(selected.length, locale),
            })}
          </Button>
        )}
      </DrawerFoot>
    </>
  );
}
