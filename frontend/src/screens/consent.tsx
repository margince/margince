import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  Skeleton,
} from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { humanizeToken } from "./audit";
import { QueryStates, throwProblem } from "./common";
import "./consent.css";
import { stable } from "../format/collate";

// The Art. 7 proof log (G-4) + the double-opt-in redeem field (G-5) for the
// Contact 360. GET /contacts/{id}/consent already returns {state, events}; this
// is the only surface that reads events — the 360 previously rendered state
// alone and silently dropped the append-only trail. requires_double_opt_in
// lives on ConsentPurpose, not on the contact's per-purpose state, so this
// section also reads GET /consent-purposes and joins on purpose_id to know
// which rows can only be confirmed by the subject through a mailed link.

type ConsentPurpose = components["schemas"]["ConsentPurpose"];
type ContactConsentState = components["schemas"]["ContactConsentState"];
type ConsentEvent = components["schemas"]["ConsentEvent"];

function useContactConsent(contactId: string) {
  return useQuery({
    queryKey: ["contact-consent", contactId],
    queryFn: async () => {
      const { data, error } = await api.GET("/contacts/{id}/consent", {
        params: { path: { id: contactId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// Same cache key settings.tsx's ConsentPurposesCard uses, so the two
// surfaces share one fetch. No pagination — the endpoint hardcodes
// has_more:false, so there is no second page to walk.
export function useConsentPurposes() {
  return useQuery({
    queryKey: ["consent-purposes"],
    queryFn: async () => {
      const { data, error } = await api.GET("/consent-purposes");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// The label for each actor kind the wire can name, keyed on the closed
// actor_type enum (audit.tsx's ACTOR_ICON is the same table shape). Keying on
// the union makes a kind added upstream a compile error here rather than a
// silent mislabel — a proof log must never assert an actor the wire did not.
const ACTOR_LABEL: Record<
  NonNullable<ConsentEvent["actor_type"]>,
  MessageKey
> = {
  human: "consent.actorHuman",
  agent: "consent.actorAgent",
  system: "consent.actorSystem",
  connector: "consent.actorConnector",
};

// The actor line on the Art. 7 proof log: names WHO the server says captured
// this decision, verbatim. This is deliberately NOT ProvenanceTag — that
// component exists for a compose/staging context ("did *you* type this, or
// an agent") and its human branch renders "typed by you" unconditionally,
// which would misattribute every human-captured grant to the viewer instead
// of the actual actor (frequently a different teammate, or the subject
// themself via a public form). A proof log's actor is evidence, not a claim
// about who is looking at it — it always shows actor_type + actor_id
// straight from the wire, never resolved against the current session.
function ConsentEventActor({ event }: Readonly<{ event: ConsentEvent }>) {
  const t = useT();
  // An event the wire never attributed is unrecorded, never a positive claim
  // about who is looking at it.
  if (!event.actor_type) {
    return <span className="t-caption">{t("consent.actorUnknown")}</span>;
  }
  const label = t(ACTOR_LABEL[event.actor_type]);
  return (
    <span className="t-caption">
      {label}
      {event.actor_id && (
        <>
          {" "}
          <span>{event.actor_id}</span>
        </>
      )}
    </span>
  );
}

// The per-purpose Art. 7 proof log: newest first, one row per transition.
// Never renders policy_text/policy_version — wireEvent never projects them
// (they're NOT NULL in consent_event but genuinely absent on the wire), so
// showing wording here would be fabricated evidence on a GDPR proof surface.
function ConsentProofLog({ events }: Readonly<{ events: ConsentEvent[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  if (events.length === 0) {
    return <EmptyState>{t("consent.proofEmpty")}</EmptyState>;
  }
  const ordered = [...events].sort((a, b) =>
    stable(b.occurred_at, a.occurred_at),
  );
  return (
    <Card as="div" inset className="consent-proof-log">
      {/* `timeline-plain`: these rows carry their date inline rather than in
          the chronicle's gutter, so they opt out of its column grid. */}
      <ul className="timeline timeline-plain">
        {ordered.map((event) => (
          <li key={event.id}>
            <span className="tl-body">
              <span className="tl-title">
                <Badge
                  tone={event.new_state === "granted" ? "success" : "warning"}
                >
                  {humanizeToken(event.new_state)}
                </Badge>{" "}
                {event.source ?? t("consent.sourceUnknown")}
              </span>
              <span className="tl-meta">
                <ConsentEventActor event={event} />
                {/* The basis this decision was argued from. It is what an
                    auditor, a subject request or a handover actually asks for
                    — "when, and on what basis" — and it was on the wire and on
                    no screen. Operator-authored free text ("Art. 6(1)(a)"),
                    not a vocabulary key, so it is shown as written rather
                    than translated. Absent stays silent: a row with no basis
                    recorded says less than one claiming a basis nobody
                    entered. */}
                {event.lawful_basis && (
                  <span>
                    {t("consent.basis", { basis: event.lawful_basis })}
                  </span>
                )}
                <span>{formatDateTime(event.occurred_at, locale, zone)}</span>
              </span>
            </span>
          </li>
        ))}
      </ul>
    </Card>
  );
}

// The state badge tone, keyed on the closed consent-state enum: unknown gets no
// tone (it isn't a withdrawal — the noRecord subtitle carries that distinction).
// Keying on the union keeps a state added upstream a compile error here rather
// than a silently untoned badge.
const STATE_TONE: Record<
  ContactConsentState["state"],
  "success" | "warning" | undefined
> = {
  granted: "success",
  withdrawn: "warning",
  unknown: undefined,
};

// One consent-purpose row on the Contact 360 (P-8/P-9): the state badge, a
// Grant/Withdraw toggle that writes an append-only consent_event through
// POST /contacts/{id}/consent, and a toggleable proof log. A purpose needing
// double opt-in says so and offers no control: only the subject can confirm
// one, from a link mailed to their own address. lawful_basis is
// intentionally omitted from the toggle body — it's optional in
// RecordConsentRequest and this control has no field for it yet. Errors
// surface verbatim (a DOI-required purpose 422s here rather than silently
// no-opping) so the human sees exactly why the toggle didn't take.
//
// A grant carries wording, because the server refuses one that cannot say what
// the subject agreed to. This door has no screen the subject read — an operator
// is recording a consent obtained elsewhere — so it submits an attestation
// naming that, rather than quoting a sentence nobody was shown.
function ConsentRow({
  mayWrite,
  contactId,
  entry,
  purpose,
  events,
}: Readonly<{
  mayWrite: boolean;
  contactId: string;
  entry: ContactConsentState;
  purpose: ConsentPurpose | undefined;
  events: ConsentEvent[];
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const granted = entry.state === "granted";
  const [showLog, setShowLog] = useState(false);
  const requiresDoi = purpose?.requires_double_opt_in ?? false;

  const setState = useMutation({
    mutationFn: async (newState: "granted" | "withdrawn") => {
      const { data, error } = await api.POST("/contacts/{id}/consent", {
        params: { path: { id: contactId } },
        body: {
          purpose_id: entry.purpose_id,
          new_state: newState,
          // Only a grant needs it: a withdrawal demonstrates nothing, and the
          // server stores no wording for one.
          ...(newState === "granted"
            ? {
                wording: t("consent.operatorWording", {
                  label: purpose?.label ?? entry.purpose_key ?? "",
                }),
              }
            : {}),
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // The write endpoint returns only the updated state row, not the new
    // consent_event — so the proof log can only pick up the transition just
    // made by refetching, not by patching the cache from this response.
    onSuccess: () => {
      for (const key of [
        "contact-consent",
        "contactConsentGuard",
        "contact360",
      ]) {
        queryClient.invalidateQueries({ queryKey: [key, contactId] });
      }
      queryClient.invalidateQueries({ queryKey: ["communication-review"] });
    },
  });

  return (
    <PanelRow className="consent-row">
      <div className="consent-row-head">
        <strong>
          {purpose?.label ?? entry.purpose_key ?? entry.purpose_id}
        </strong>
        <Badge tone={STATE_TONE[entry.state]}>
          {humanizeToken(entry.state)}
        </Badge>
        {entry.state === "unknown" && (
          <span className="t-caption">{t("consent.noRecord")}</span>
        )}
        {/* The basis the CURRENT state stands on, beside the state itself. The
            log below says how the record got here; this says what holds now,
            which is the half a reader acting today needs. */}
        {entry.lawful_basis && (
          <span className="t-caption">
            {t("consent.basis", { basis: entry.lawful_basis })}
          </span>
        )}
      </div>
      <div className="consent-row-actions">
        {/* Withdraw stays on every row: a contact may always take consent back,
            and a double-opt-in purpose is no exception. Granting one from here
            is what disappears — the server refuses it, because only the subject
            can confirm a purpose that requires the round trip, so offering the
            button would promise something every click fails to do. */}
        {mayWrite && (granted || !requiresDoi) && (
          <Button
            disabled={setState.isPending}
            onClick={() => setState.mutate(granted ? "withdrawn" : "granted")}
          >
            {granted ? t("consent.withdraw") : t("consent.grant")}
          </Button>
        )}
        <Button onClick={() => setShowLog((value) => !value)}>
          {t("consent.proofLog")}
        </Button>
      </div>
      {requiresDoi && <p>{t("consent.doiBySubject")}</p>}
      <ErrorLine error={setState.error} />
      {showLog && <ConsentProofLog events={events} />}
    </PanelRow>
  );
}

export function ConsentSection({
  contactId,
  contact,
  showConfirm = true,
  titleLevel = 2,
}: Readonly<{
  contactId: string;
  contact?: { readonly writable?: boolean };
  showConfirm?: boolean;
  titleLevel?: 2 | 3;
}>) {
  // Every verb in this section writes to the CONTACT, so they share one
  // decision: the role's grant and this row's own `writable`. Absent fails
  // closed, which is what a section rendered before its record has loaded
  // should do — an editor drawn on a maybe is a control the save refuses.
  const mayWrite = useCanWriteRecord("contact", contact);
  const t = useT();
  const consentQuery = useContactConsent(contactId);
  const purposesQuery = useConsentPurposes();
  const purposes = purposesQuery.data?.data ?? [];
  // Only trust "no purposes" once the purposes fetch itself has actually
  // succeeded — while it's still pending this would otherwise flash the
  // empty state before the real list ever arrives.
  const noPurposes = purposesQuery.isSuccess && purposes.length === 0;
  const consent = consentQuery.data;

  // requires_double_opt_in lives only on ConsentPurpose, so a row's DOI gate
  // depends on purposesQuery having actually succeeded — a failed fetch that
  // fell back to `[]` here (rather than erroring loudly) would make every
  // DOI-required purpose render as freely grantable, silently dropping a
  // legal control. share.tsx's RosterPicker gates its two roster fetches the
  // same explicit way, for the same reason: a collapsed-to-empty failure
  // must never be mistaken for a real empty list.
  // Until the consent read settles the panel shows that read's own state, and
  // every state but the settled one is prose that wants the body's margin. The
  // purposes themselves are rows of a list and run to the panel's edges.
  let body: ReactNode = (
    <PanelBody>
      <QueryStates query={consentQuery} pendingLabel={t("contact.consent")}>
        {null}
      </QueryStates>
    </PanelBody>
  );
  if (consent) {
    if (purposesQuery.isPending) {
      body = (
        <PanelBody>
          <Skeleton width="60%" />
        </PanelBody>
      );
    } else if (purposesQuery.isError) {
      body = (
        <PanelBody>
          <EmptyState>
            <p>{t("consent.purposesUnavailable")}</p>
            <Button onClick={() => purposesQuery.refetch()}>
              {t("common.retry")}
            </Button>
          </EmptyState>
        </PanelBody>
      );
    } else if (noPurposes) {
      body = (
        <PanelBody>
          <EmptyState>{t("consent.noPurposes")}</EmptyState>
        </PanelBody>
      );
    } else {
      body = consent.state.map((entry) => (
        <ConsentRow
          mayWrite={mayWrite}
          key={entry.purpose_id}
          contactId={contactId}
          entry={entry}
          purpose={purposes.find((purpose) => purpose.id === entry.purpose_id)}
          events={consent.events.filter(
            (event) => event.purpose_id === entry.purpose_id,
          )}
        />
      ));
    }
  }

  return (
    <Panel title={t("contact.consent")} titleLevel={titleLevel}>
      <PanelBody>
        <p className="t-sub">{t("consent.defaultDeny")}</p>
      </PanelBody>
      {body}
      {/* Keyed on the contact: a mutation result is about the record it was
          asked for, and React would otherwise reuse this component across a
          navigation between two cached contacts and leave the previous
          contact's address sitting under the new record. */}
      {showConfirm && (
        <ConfirmDetailsAction
          key={contactId}
          contactId={contactId}
          mayWrite={mayWrite}
        />
      )}
    </Panel>
  );
}

// Issuing the link queues a message; it does not prove delivery.
function sentenceFor(
  issued: components["schemas"]["ConfirmRequestIssued"],
  t: ReturnType<typeof useT>,
): string {
  const address = issued.delivered_to;
  return issued.queued
    ? t("consent.askQueued", { address })
    : t("consent.askNotDelivered", { address });
}

/**
 * ConfirmDetailsAction mails the contact a link to see what is held about them,
 * correct it, and answer on marketing.
 *
 * The address is never chosen here. The server derives it from the contact's own
 * live primary email, which is what lets a grant made through the link stand on
 * its own: the answer came from the subject's mailbox. So this surface offers
 * the act and reports where it went, and cannot aim it anywhere.
 */
export function ConfirmDetailsAction({
  contactId,
  mayWrite,
  recipient,
  unavailableReason,
}: Readonly<{
  contactId: string;
  mayWrite: boolean;
  recipient?: string | null;
  unavailableReason?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const ask = useMutation({
    // Keyed on the contact, so a result belongs to the record it was asked
    // about. React reuses this component across a navigation between two
    // cached contacts, and without the key the previous contact's address sat
    // under the new record's rows — naming somebody else's mailbox as the one
    // this contact's link went to.
    mutationKey: ["confirm-request", contactId],
    mutationFn: async (id: string) => {
      const { data, error } = await api.POST(
        "/contacts/{id}/consent/confirm-request",
        { params: { path: { id } } },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  if (!mayWrite) {
    return null;
  }
  return (
    <PanelBody className="consent-confirm-ask">
      <Button
        disabled={ask.isPending}
        reason={unavailableReason}
        data-testid="confirm-details-ask"
        onClick={() => ask.mutate(contactId)}
      >
        {t("consent.askToConfirm")}
      </Button>
      {recipient && (
        <p className="t-caption">
          {t("consent.confirmRecipient", { address: recipient })}
        </p>
      )}
      <p>{t("consent.askToConfirmWhat")}</p>
      <ErrorLine error={ask.error} />
      {ask.data && (
        <p className="t-caption" data-testid="confirm-details-sent">
          {sentenceFor(ask.data, t)} {t("consent.askExpires")}:{" "}
          {formatDateTime(ask.data.expires_at, locale, zone)}
        </p>
      )}
    </PanelBody>
  );
}
