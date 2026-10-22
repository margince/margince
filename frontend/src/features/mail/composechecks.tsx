// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  ChevronUp,
  CircleCheck,
  type LucideIcon,
  ShieldAlert,
} from "lucide-react";
import { type ReactNode, useState } from "react";
import type { components } from "../../api/schema";
import {
  decidingRecipient,
  SendPermission,
} from "../../components/sendpermission";
import { Heading } from "../../design-system/heading";
import { Popover } from "../../design-system/popover";
import { formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  asksWhy,
  type CommunicationContext,
  contextOptions,
} from "../../lib/compose-context";
import type { Activity } from "./compose";
import { MailSendNotices, sharedUnsubscribeAhead } from "./compose.parts";
import type { ComposeFields } from "./composefields";
import { ContactReasonForm, type ReasonTarget } from "./composereason";
import { momentLabel, momentOf } from "./composeschedule";
import type { ComposeSend } from "./composesend";

type Preview = components["schemas"]["SendAuthorizationPreview"];

/** What the foot says about the checks at a glance, before they are opened. */
export type ChecksVerdict =
  | Readonly<{ state: "checking" }>
  | Readonly<{ state: "blocked"; count: number }>
  | Readonly<{ state: "ready" }>
  | Readonly<{ state: "unasked" }>;

/**
 * The checks a send must pass, counted.
 *
 * Only what a press of Send would meet. That is a reason for contact still
 * unchosen, an engine answer that refuses, or a marketing send to two people. Ready is said only on an engine answer that allows. A message
 * nobody has asked about yet is not one the engine passed.
 */
export function checksVerdict(
  input: Readonly<{
    permission: ComposeSend["permission"];
    missingReason: boolean;
    sharedUnsubscribe: boolean;
  }>,
): ChecksVerdict {
  const { state } = decidingRecipient(
    input.permission.preview,
    input.permission.asking,
  );
  if (state === "checking") return { state: "checking" };
  const count =
    (state === "refused" || state === "unproven" ? 1 : 0) +
    (input.missingReason ? 1 : 0) +
    (input.sharedUnsubscribe ? 1 : 0);
  if (count > 0) return { state: "blocked", count };
  if (input.permission.preview && !input.permission.unanswered) {
    return { state: "ready" };
  }
  return { state: "unasked" };
}

/**
 * Who a vouch for the deciding recipient would be recorded on. That is known
 * when the engine says one may be and the composer knows the contact.
 *
 * Absent otherwise, and the refusal then renders without the control: a form
 * that could only answer 404 is the dead button `SendPermission` refuses to draw.
 */
export function reasonTargetOf(
  preview: Preview | undefined,
  contactOf: (address: string) => string | undefined,
): ReasonTarget | undefined {
  const { state, recipient } = decidingRecipient(preview);
  if (state !== "unproven" || !recipient?.resolved_category) {
    return undefined;
  }
  const contactId = contactOf(recipient.address);
  if (!contactId) {
    return undefined;
  }
  return {
    contactId,
    address: recipient.address,
    category: recipient.resolved_category,
  };
}

function CheckRow({
  icon: Icon,
  tone,
  children,
}: Readonly<{
  icon: LucideIcon;
  tone: "success" | "warning";
  children: ReactNode;
}>) {
  return (
    <div className="compose-check" data-tone={tone}>
      <Icon aria-hidden="true" />
      <span>{children}</span>
    </div>
  );
}

// Why the message may be written, as the form answered it: the thread it
// replies to, or the reason the reader chose under the subject.
function PurposeRow({
  anchorActivity,
  context,
}: Readonly<{
  anchorActivity: Activity | undefined;
  context: CommunicationContext | "";
}>) {
  const t = useT();
  if (!asksWhy(anchorActivity)) {
    return (
      <CheckRow icon={CircleCheck} tone="success">
        {t("compose.derivedReply")}
      </CheckRow>
    );
  }
  const chosen = contextOptions(t).find((option) => option.value === context);
  if (!context || !chosen) {
    return (
      <CheckRow icon={ShieldAlert} tone="warning">
        {t("compose.missingWhy")}
      </CheckRow>
    );
  }
  return (
    <CheckRow icon={CircleCheck} tone="success">
      {t("compose.checksWhy", { reason: chosen.label })}
    </CheckRow>
  );
}

function TriggerLabel({ verdict }: Readonly<{ verdict: ChecksVerdict }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (verdict.state === "blocked") {
    return (
      <>
        <ShieldAlert aria-hidden="true" />
        {plural("compose.checksBlocked", verdict.count, {
          count: formatNumber(verdict.count, locale),
        })}
        <ChevronUp aria-hidden="true" />
      </>
    );
  }
  if (verdict.state === "ready") {
    return (
      <>
        <CircleCheck aria-hidden="true" />
        {t("sendPermission.ready")}
      </>
    );
  }
  return (
    <>
      {t(
        verdict.state === "checking"
          ? "compose.checksChecking"
          : "compose.checks",
      )}
      <ChevronUp aria-hidden="true" />
    </>
  );
}

/**
 * Every check a send meets, in one list opened from the foot.
 *
 * The foot says how many still stand in the way, and the list says which. It
 * offers what can be done about each where it is read. That is the reason for
 * contact, and a reason to write recorded under the rep's own name.
 */
export function SendChecks({
  fields,
  send,
  anchorActivity,
  contactOf,
  bodyKey,
  open,
  onOpenChange,
}: Readonly<{
  fields: ComposeFields;
  send: ComposeSend;
  anchorActivity: Activity | undefined;
  /** The contact an address belongs to, where the records behind the draft say. */
  contactOf: (address: string) => string | undefined;
  bodyKey: MessageKey;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // Whether the reader asked to record a reason, which the refusal offers.
  const [vouching, setVouching] = useState(false);
  const permission = send.permission;
  const target = reasonTargetOf(permission.preview, contactOf);
  const verdict = checksVerdict({
    permission,
    missingReason: asksWhy(anchorActivity) && fields.context === "",
    sharedUnsubscribe: sharedUnsubscribeAhead(
      fields.to,
      fields.cc,
      send.claimedContext,
    ),
  });
  const scheduled = momentOf(fields.sendAt);
  return (
    <Popover
      label={<TriggerLabel verdict={verdict} />}
      variant="ghost"
      className={`compose-checks-trigger is-${verdict.state}`}
      panelClassName="compose-checks"
      dialog
      open={open}
      onOpenChange={onOpenChange}
    >
      <Heading size="xsmall" as="h2">
        {t("compose.checks")}
      </Heading>
      {fields.to.length > 0 ? (
        <CheckRow icon={CircleCheck} tone="success">
          {t("compose.checksGoesTo", { addresses: fields.to.join(", ") })}
        </CheckRow>
      ) : (
        <CheckRow icon={ShieldAlert} tone="warning">
          {t("compose.emptyRecipients")}
        </CheckRow>
      )}
      <PurposeRow anchorActivity={anchorActivity} context={fields.context} />
      <MailSendNotices
        to={fields.to}
        cc={fields.cc}
        context={send.claimedContext}
      />
      <SendPermission
        preview={permission.preview}
        asking={permission.asking}
        unanswered={permission.unanswered}
        onOverride={target ? () => setVouching(true) : undefined}
      />
      {target && vouching && (
        <ContactReasonForm
          key={`${target.contactId}:${target.category}`}
          target={target}
          onRecorded={() => setVouching(false)}
        />
      )}
      {verdict.state === "ready" && (
        <CheckRow icon={CircleCheck} tone="success">
          {t("compose.checksAllowed")}
        </CheckRow>
      )}
      {scheduled && (
        <p className="t-caption">
          {t("compose.willGoOut", {
            when: momentLabel(scheduled, locale, viewerZone()),
          })}
        </p>
      )}
      <p className="t-caption compose-checks-foot">{t(bodyKey)}</p>
    </Popover>
  );
}
