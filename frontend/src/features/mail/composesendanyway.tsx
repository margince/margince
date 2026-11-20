// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
  useEffect,
  useId,
  useRef,
} from "react";
import type { components } from "../../api/schema";
import { CitedLaws, FindingWithLaw } from "../../components/citedlaw";
import { lawsOf } from "../../components/citedprovisions";
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
import { addressKey } from "../../lib/addresskey";
import { ComposeOutcome } from "./compose.parts";
import {
  INLINE_WARNING_VERSION,
  type InlineReview,
} from "./composeinlinereview";
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
    review.refusals.map((refusal) => addressKey(refusal.address)),
  );
  const values = new Set(
    lines.filter((address) => named.has(addressKey(address))),
  );
  return { values, label: t("compose.sendAnyway.marked") };
}

/**
 * The band above the foot when the send is the check: what the send came back
 * with, and nothing asked while the rep writes.
 */
export function InlineBlockers({
  isChannelReply,
  refusal,
  sendUnavailable,
  bodyKey,
  children,
  ...rows
}: Readonly<{
  isChannelReply: boolean;
  refusal: Refusal;
  sendUnavailable: boolean;
  bodyKey: MessageKey;
  state: InlineReview;
  explanation: string;
  onExplanationChange: (text: string) => void;
  /** What the message still needs before Send, such as its reason. */
  children?: ReactNode;
}>) {
  return (
    <div className="compose-blockers">
      {children}
      <ComposeOutcome
        isChannelReply={isChannelReply}
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
          lead ? (
            <FindingWithLaw
              sentence={t(findingKey(lead.reason_code), {
                address: lead.address,
              })}
              provisions={lawsOf(lead)}
            />
          ) : (
            t("compose.sendAnyway.finding.none")
          )
        }
      >
        {lead && <LeadCountry refusal={lead} />}
        {!state.canSendAnyway && <p>{t("compose.sendAnyway.editOnly")}</p>}
        {state.changed && <p>{t("directSend.changed")}</p>}
        <MoreFindings refusals={review.refusals.slice(1)} />
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

// The findings after the leading one, behind one count. The leading finding
// is already the warning's own text, so it is not listed again.
function MoreFindings({
  refusals,
}: Readonly<{ refusals: readonly RefusedRecipient[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (refusals.length === 0) return null;
  return (
    <Disclosure
      summary={plural("compose.sendAnyway.more", refusals.length, {
        count: formatNumber(refusals.length, locale),
      })}
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
    ...lawsOf(refusal),
    ...(refusal.footer_findings ?? []).flatMap(lawsOf),
  ];
}

// The country the leading refusal was judged under and where it was read. The
// rep sees why this law applies and can correct it on the chip.
function LeadCountry({ refusal }: Readonly<{ refusal: RefusedRecipient }>) {
  const t = useT();
  const { locale } = useLocale();
  const country = refusalCountryText(refusal, t, locale);
  return country ? <p>{country}</p> : null;
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
        // The served words, verbatim: the decision records that these were shown.
        label={servedQuestion(state)}
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

// The question the review was served with for this composer. Send anyway is
// offered only where it was served, so it is always there when the row is.
function servedQuestion(state: InlineReview): string {
  return (
    state.review?.warnings?.find(
      (warning) => warning.version === INLINE_WARNING_VERSION,
    )?.text ?? ""
  );
}
