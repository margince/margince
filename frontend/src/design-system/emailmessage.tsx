// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Fragment, type ReactNode } from "react";

import { api } from "../api/client";
import type { components } from "../api/schema";
import type { ActivityWhenFormatter } from "../format/activitywhen";
import { formatBytes, formatNumber } from "../format/format";
import { translatePlural, useLocale, useT } from "../i18n";
import { ENTITY } from "../lib/entity";
import { routeHash } from "../lib/router";
import { FileChip } from "./filechip";
import { relayMemberFor, relayProvenance } from "./provenanceof";
import type { Provenance } from "./trust";
import "./emaildetail.css";

// One message's parts, shared by the drawer, the thread inside it and the
// task panel's source reading.

type EmailPresentation = components["schemas"]["EmailPresentation"];
type EmailParty = components["schemas"]["EmailParty"];
type EmailAttachmentSummary = components["schemas"]["EmailAttachmentSummary"];

/**
 * The key a message's read is cached under. The audience writes refresh it
 * after changing who may read the message, so it is spelled once.
 */
export function emailDetailKey(activityId: string) {
  return ["email-presentation", activityId] as const;
}

/**
 * One message's canonical read. A message's content is an authorization
 * result, so every caller asks again rather than trusting a cached answer.
 */
export async function readEmailPresentation(
  activityId: string,
): Promise<EmailPresentation> {
  const { data, error } = await api.GET("/activities/{id}/email-presentation", {
    params: { path: { id: activityId } },
  });
  if (error) {
    throw error;
  }
  return data;
}

/**
 * What came with the message, as the `FileChip` cards the file lists draw.
 * Callers draw it only for a readable message: which files arrived is a fact
 * about the message too.
 */
export function EmailAttachments({
  files,
}: Readonly<{ files: readonly EmailAttachmentSummary[] }>) {
  const { locale } = useLocale();
  const t = useT();
  if (files.length === 0) {
    // No empty region: a heading over nothing says the message had files and
    // they are missing, which is a different claim from having had none.
    return null;
  }
  return (
    <div className="emaildetail__files">
      <p>
        {translatePlural(locale, "email.detail.attachments", files.length, {
          count: formatNumber(files.length, locale),
        })}
      </p>
      <ul className="emaildetail__fileList">
        {files.map((file) => (
          <li key={file.id}>
            <FileChip
              // A private message's file was named but not kept, so it is
              // listed without a link to bytes that do not exist.
              {...(file.bytes_withheld
                ? { withheld: t("email.detail.attachmentWithheld") }
                : { href: `/v1/attachments/${file.id}` })}
              filename={file.filename}
              // Absent rather than zero when the server sent no size: a size
              // it could not record is not a file of no bytes.
              size={
                file.byte_size == null
                  ? undefined
                  : formatBytes(file.byte_size, locale)
              }
            />
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * What one participant is called: the name, then the address, then nothing.
 * `||` rather than `??`, so an empty name falls through to the address.
 */
export function partyName(party: EmailParty): string {
  return party.display_name?.trim() || party.address.trim();
}

/**
 * One participant, named, and a link to their contact when the server
 * resolved the address to one this reader may see. The link opens a new tab,
 * so the message being read stays open.
 */
function PartyName({ party }: Readonly<{ party: EmailParty }>) {
  const name = partyName(party);
  if (!party.contact_id) {
    return <>{name}</>;
  }
  return (
    <a
      className="entity-link"
      href={routeHash(ENTITY.contact.route(party.contact_id))}
      // `rel` travels with `target`, never behind it: a blank target without
      // `noopener` hands the opened page a live handle back into this one.
      target="_blank"
      rel="noopener noreferrer"
    >
      {name}
    </a>
  );
}

/**
 * One role's participants on one envelope line: From, To or Cc. A party with
 * neither name nor address is dropped, and a line left empty draws nothing.
 */
function PartyLine({
  label,
  parties,
}: Readonly<{ label: string; parties: EmailParty[] }>) {
  const named = parties.filter((party) => partyName(party) !== "");
  if (named.length === 0) {
    return null;
  }
  return (
    <p className="emaildetail__party">
      <span className="emaildetail__partyLabel">{label}</span>
      {/* Separators go between the elements; a `join` would stringify them. */}
      {named.map((party, index) => (
        // Keyed as the participant table keys a row: one address can be a
        // contact and a seat on the same line.
        <Fragment
          key={`${party.address}|${party.contact_id ?? ""}|${party.user_id ?? ""}`}
        >
          {index > 0 && ", "}
          <PartyName party={party} />
        </Fragment>
      ))}
    </p>
  );
}

/**
 * Who the message presents as its sender. A relay send names the relay's own
 * address, never the member it was sent for.
 */
export function senderParties(presentation: EmailPresentation): EmailParty[] {
  const send = presentation.controller_send;
  if (!send) {
    return presentation.from;
  }
  return send.from_address ? [{ address: send.from_address }] : [];
}

/** The relay arm of a relay send, by the History row's rule; null otherwise. */
export function relaySendOf(
  presentation: EmailPresentation,
  viewerUserId: string | undefined,
): Provenance | null {
  const send = presentation.controller_send;
  if (!send) {
    return null;
  }
  const member = presentation.from.find((party) => party.user_id)?.user_id;
  return relayProvenance(send, relayMemberFor(member, viewerUserId));
}

/**
 * The envelope: who the message was with and when it was sent. The email
 * drawer adds its filing line as `children`; the task panel draws it bare.
 */
export function EmailEnvelope({
  presentation,
  formatWhen,
  children,
}: Readonly<{
  presentation: EmailPresentation;
  formatWhen: ActivityWhenFormatter;
  children?: ReactNode;
}>) {
  const t = useT();
  return (
    <div className="emaildetail__parties">
      <PartyLine
        label={t("email.detail.from")}
        parties={senderParties(presentation)}
      />
      <PartyLine label={t("email.detail.to")} parties={presentation.to} />
      <PartyLine label={t("email.detail.cc")} parties={presentation.cc} />
      <p className="emaildetail__party">
        <span className="emaildetail__partyLabel">
          {t("email.detail.when")}
        </span>
        {formatWhen(presentation.occurred_at, presentation.occurred_precision)}
      </p>
      {/* Said, not shown: no Bcc line would read as nobody blind-copied. */}
      {presentation.bcc_withheld && (
        <p className="emaildetail__party emaildetail__party--withheld">
          {t("email.detail.bccWithheld")}
        </p>
      )}
      {children}
    </div>
  );
}
