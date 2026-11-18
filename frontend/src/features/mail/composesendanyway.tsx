// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type KeyboardEvent,
  type MouseEvent,
  useEffect,
  useId,
  useRef,
} from "react";
import type { components } from "../../api/schema";
import { Button, Field, Textarea } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { describedByIds } from "../../design-system/describedby";
import { Disclosure } from "../../design-system/disclosure";
import { ErrorLine } from "../../design-system/errorline";
import { footerFindingsText } from "../../format/footerfindings";
import { formatNumber } from "../../format/format";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { CitedAttention, CitedLaws } from "./citedlaw";
import { ComposeOutcome } from "./compose.parts";
import type { InlineReview } from "./composeinlinereview";
import type { Refusal } from "./composerefusal";
import { refusalCountryText } from "./refusalcountry";
import "./compose.css";

type RefusedRecipient =
  components["schemas"]["CommunicationReview"]["refusals"][number];

// One short sentence per reason code. An unknown code gets the factual
// fallback, never the code itself.
const FINDING_KEYS: Readonly<Record<string, MessageKey>> = {
  marketing_objection: "compose.sendAnyway.finding.objected",
  consent_withdrawn: "compose.sendAnyway.finding.withdrawn",
  subject_request: "compose.sendAnyway.finding.askedToStop",
  processing_restricted: "compose.sendAnyway.finding.restricted",
  opted_out_after_request: "compose.sendAnyway.finding.optedOut",
  no_compatible_evidence: "compose.sendAnyway.finding.noReason",
  deal_not_requested: "compose.sendAnyway.finding.noReason",
  no_marketing_consent: "compose.sendAnyway.finding.noPermission",
  marketing_permission_required: "compose.sendAnyway.finding.noPermission",
  marketing_consent_required: "compose.sendAnyway.finding.noPermission",
  unconfirmed_double_opt_in: "compose.sendAnyway.finding.noPermission",
  footer_incomplete: "compose.sendAnyway.finding.footer",
  hard_bounce: "compose.sendAnyway.finding.bounced",
  frequency_cap_reached: "compose.sendAnyway.finding.tooMany",
  lawful_basis_unconfirmed: "compose.sendAnyway.finding.lawfulBasis",
};

export function findingKey(reasonCode: string): MessageKey {
  return FINDING_KEYS[reasonCode] ?? "compose.sendAnyway.finding.other";
}

/**
 * The chips to mark: the addresses the review names, as the composer's
 * recipient lines spell them. Matched without case, as the server
 * canonicalizes an address. None while no review is shown.
 */
export function markedRecipients(
  review: InlineReview["review"],
  fields: Readonly<{ to: string[]; cc: string[]; bcc: string[] }>,
  t: Translator,
): Readonly<{ values: ReadonlySet<string>; label: string }> | undefined {
  if (!review) return undefined;
  const lines = [...fields.to, ...fields.cc, ...fields.bcc];
  const named = new Set(
    review.refusals.map((refusal) => refusal.address.toLowerCase()),
  );
  const values = new Set(
    lines.filter((address) => named.has(address.trim().toLowerCase())),
  );
  return { values, label: t("compose.sendAnyway.marked") };
}

/**
 * The band above the foot when the send is the check: what the send came back
 * with, and nothing asked while the rep writes.
 */
export function InlineBlockers({
  refusal,
  sendUnavailable,
  bodyKey,
  ...rows
}: Readonly<{
  refusal: Refusal;
  sendUnavailable: boolean;
  bodyKey: MessageKey;
  state: InlineReview;
  explanation: string;
  onExplanationChange: (text: string) => void;
}>) {
  return (
    <div className="compose-blockers">
      <ComposeOutcome
        isChannelReply={false}
        bodyKey={bodyKey}
        sendUnavailable={sendUnavailable}
        refusal={refusal}
      />
      <SendAnywayRows {...rows} />
    </div>
  );
}

/**
 * What a refused Send left in the composer: the server's leading finding with
 * the short warning. Where the server lets this sender decide, one reason and
 * Send anyway cover the whole message.
 */
export function SendAnywayRows({
  state,
  explanation,
  onExplanationChange,
}: Readonly<{
  state: InlineReview;
  explanation: string;
  onExplanationChange: (text: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const warningId = useId();
  if (!state.shown) return null;
  if (state.loading) {
    return (
      <p className="t-caption" role="status">
        {t("compose.sendAnyway.loading")}
      </p>
    );
  }
  if (state.loadFailed) {
    return (
      <ErrorLine
        actions={
          <Button variant="link" onClick={state.retryLoad}>
            {t("compose.sendAnyway.retry")}
          </Button>
        }
      >
        {t("compose.sendAnyway.loadFailed")}
      </ErrorLine>
    );
  }
  if (state.unreviewed || !state.review) {
    return (
      <Callout
        tone="warning"
        live="status"
        title={t("compose.sendAnyway.unreviewed")}
      />
    );
  }
  const { review } = state;
  const lead = review.refusals[0];
  return (
    <>
      <Callout
        tone="warning"
        live="status"
        id={warningId}
        title={
          lead?.provision ? (
            <CitedAttention address={lead.address} provision={lead.provision} />
          ) : lead ? (
            t(findingKey(lead.reason_code), { address: lead.address })
          ) : (
            t("compose.sendAnyway.finding.none")
          )
        }
      >
        {lead?.provision && (
          <p>{t(findingKey(lead.reason_code), { address: lead.address })}</p>
        )}
        <p>
          {t(
            state.canSendAnyway
              ? "compose.sendAnyway.warning"
              : "compose.sendAnyway.editOnly",
          )}
        </p>
        {lead && refusalCountryText(lead, t, locale) && (
          <p>{refusalCountryText(lead, t, locale)}</p>
        )}
        {state.changed && <p>{t("directSend.changed")}</p>}
        <EveryFinding refusals={review.refusals} />
      </Callout>
      {state.canSendAnyway && (
        <ReasonRow
          key={`${review.id}:${review.assessment_revision ?? ""}`}
          warningId={warningId}
          explanation={explanation}
          onExplanationChange={onExplanationChange}
          state={state}
        />
      )}
      {state.failed && <ErrorLine>{t("compose.sendAnyway.failed")}</ErrorLine>}
    </>
  );
}

// The country a refusal was judged under, after its finding, or nothing.
function withCountry(
  refusal: RefusedRecipient,
  t: Translator,
  locale: Locale,
): string | null {
  const country = refusalCountryText(refusal, t, locale);
  return country ? ` ${country}` : null;
}

// Every finding, and every message-wide footer finding, behind one count.
function EveryFinding({
  refusals,
}: Readonly<{ refusals: readonly RefusedRecipient[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const more = refusals.length - 1;
  const footers = refusals.some((refusal) => refusal.footer_findings?.length);
  if (more < 1 && !footers) return null;
  return (
    <Disclosure
      summary={
        more > 0
          ? plural("compose.sendAnyway.more", more, {
              count: formatNumber(more, locale),
            })
          : t("compose.sendAnyway.details")
      }
    >
      <ul>
        {refusals.map((refusal) => (
          <li
            key={`${refusal.address}-${refusal.subject_id ?? ""}-${refusal.reason_code}`}
          >
            {t(findingKey(refusal.reason_code), { address: refusal.address })}
            {withCountry(refusal, t, locale)}
            {refusal.footer_findings?.length
              ? ` ${footerFindingsText(refusal.footer_findings, t, locale)}`
              : null}{" "}
            <CitedLaws provisions={provisionsOf(refusal)} />
          </li>
        ))}
      </ul>
    </Disclosure>
  );
}

// The law the refusal and each of its footer findings cite, where any does.
function provisionsOf(refusal: RefusedRecipient) {
  return [
    refusal.provision,
    ...(refusal.footer_findings ?? []).map((finding) => finding.provision),
  ];
}

// The reason and the button. Keyed by the review's revision, so a refreshed
// review mounts a new button that needs a press of its own.
function ReasonRow({
  warningId,
  explanation,
  onExplanationChange,
  state,
}: Readonly<{
  warningId: string;
  explanation: string;
  onExplanationChange: (text: string) => void;
  state: InlineReview;
}>) {
  const t = useT();
  const reasonRef = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    reasonRef.current?.focus();
  }, []);
  const blank = explanation.trim() === "";
  return (
    <div className="compose-sendanyway-row">
      <Field
        label={t("compose.sendAnyway.reasonLabel")}
        className="compose-sendanyway-reason"
        required
      >
        {(control) => (
          // A textarea, so Enter types a line and never confirms.
          <Textarea
            {...control}
            ref={reasonRef}
            aria-describedby={describedByIds(
              warningId,
              control["aria-describedby"],
            )}
            grow
            maxLength={1000}
            value={explanation}
            onChange={(event) => onExplanationChange(event.target.value)}
          />
        )}
      </Field>
      <FreshPressButton
        reason={blank ? t("directSend.needsReason") : undefined}
        pending={state.pending}
        onPress={() => state.confirm(explanation)}
      >
        {t("compose.sendAnyway.confirm")}
      </FreshPressButton>
    </div>
  );
}

/**
 * A button that answers only a press begun on it. The Send press that brought
 * it up started before it was drawn. So did a double click's second half and
 * a held key's repeat, and none of them reaches `onPress`. A click with no pointer,
 * as a screen reader sends, counts while the button has focus.
 */
function FreshPressButton({
  reason,
  pending,
  onPress,
  children,
}: Readonly<{
  reason: string | undefined;
  pending: boolean;
  onPress: () => void;
  children: string;
}>) {
  const begun = useRef<"pointer" | "key" | "repeat" | null>(null);
  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      begun.current = event.repeat ? "repeat" : "key";
    }
  };
  const onClick = (event: MouseEvent<HTMLButtonElement>) => {
    const how = begun.current;
    begun.current = null;
    const assistive =
      how === null &&
      event.detail === 0 &&
      event.currentTarget === document.activeElement;
    if (how === "pointer" || how === "key" || assistive) onPress();
  };
  return (
    <Button
      variant="danger"
      reason={reason}
      pending={pending}
      onMouseDown={(event) => {
        // A second or third click of one burst is the burst that pressed Send.
        begun.current = event.detail > 1 ? null : "pointer";
      }}
      onKeyDown={onKeyDown}
      onClick={onClick}
    >
      {children}
    </Button>
  );
}
