// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { Paperclip, X } from "lucide-react";
import { type ReactNode, useId } from "react";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { splitEmailBody } from "../format/emailtext";
import { formatBytes, formatNumber } from "../format/format";
import { translatePlural, useLocale, useT } from "../i18n";
import { Button, Modal } from "./atoms";
import { SurfaceState } from "./surfacestate";
import "./emaildetail.css";

// One email, read whole, in the drawer form of the shared Modal.
//
// A drawer rather than a centred dialog because a reader opens a message FROM
// a record and is still working on that record: the account stays legible
// behind it, which is what the right placement is for. On a phone the same
// Modal is a full-screen sheet, so nothing here decides that.
//
// It fetches on open and never before. A timeline row draws from the summary
// its list already carried, so a page of twenty rows costs one request; asking
// for a message nobody opened would undo exactly that.

type EmailPresentation = components["schemas"]["EmailPresentation"];
type EmailParty = components["schemas"]["EmailParty"];
type EmailAttachmentSummary = components["schemas"]["EmailAttachmentSummary"];

/**
 * The key a message's canonical read is cached under. Exported because the
 * audience writes have to refresh it after changing who may read the message,
 * and a key spelled twice is a drawer that goes stale.
 */
export function emailDetailKey(activityId: string) {
  return ["email-presentation", activityId] as const;
}

export function EmailDetail({
  activityId,
  onClose,
  formatWhen,
  renderAccess,
  renderAccessMarkers,
}: Readonly<{
  activityId: string;
  onClose: () => void;
  /** The caller owns the reader's timezone, so it owns the formatting. */
  formatWhen: (iso: string) => string;
  /**
   * What this message's access IS, as markers beside the subject.
   *
   * Separate from `renderAccess` because the two answer different questions at
   * different moments: this is the glance a reader takes before reading, and
   * that is the sentence and the control they reach for after. Splitting them
   * is also what keeps the header short — a paragraph beside a subject line
   * pushes the message itself below the fold.
   */
  renderAccessMarkers?: (access: EmailPresentation["access"]) => ReactNode;
  /**
   * Who reads this message, and the control to change it.
   *
   * Passed in rather than mounted here because the editor performs WRITES: it
   * reaches the audience service and the roster reads, which live in `screens/`
   * where the app's queries do. A design-system component importing those would
   * turn the catalog into a layer that talks to the API.
   *
   * Optional, so a story or a preview can draw the message without wiring the
   * writes — and absent means no region, never an empty one.
   */
  renderAccess?: (presentation: EmailPresentation) => ReactNode;
}>) {
  const t = useT();
  // Generated rather than fixed: two drawers mounted at once would otherwise
  // share an id, and a dialog labelled by a duplicate is labelled by whichever
  // one the browser found first.
  const titleId = useId();
  const read = useQuery({
    queryKey: emailDetailKey(activityId),
    // A message's content is an AUTHORIZATION result, not a value that ages.
    // The global 30-second staleTime would let a reopen skip the request
    // entirely, and the default gcTime would let it paint the last open's
    // subject and body while a refetch ran — both of which show a reader what
    // they WERE allowed to see rather than what they are, and an audience
    // narrowed by somebody else cannot invalidate this browser's cache at all.
    //
    // So: ask every time, and keep nothing to repaint. leadkeys.ts documents
    // the same hazard for the promote preview and says plainly that
    // invalidation does not purge an inactive query's data; the answer there
    // was to state it, and the answer here has to be stronger, because what
    // this one would repaint is somebody's mail.
    staleTime: 0,
    gcTime: 0,
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/activities/{id}/email-presentation",
        { params: { path: { id: activityId } } },
      );
      if (error) {
        throw error;
      }
      return data;
    },
  });

  // The status decides FIRST. Reaching for the subject and falling back to the
  // withheld wording only when it is empty would print a subject that a
  // response assembled by a path which forgot to strip it still carried.
  const title = !read.data
    ? t("email.detail.none")
    : read.data.access.content_state === "withheld"
      ? t("email.withheldSubject")
      : read.data.summary.subject?.trim() || t("email.noSubject");

  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={titleId}
      placement="right"
      size="wide"
    >
      {/* A visible way out. On a phone the drawer is the whole viewport, so
          there is no backdrop to tap and usually no Escape key — the trap the
          Modal builds for keyboard users becomes a trap in the ordinary sense
          without this. */}
      <div className="emaildetail__head">
        <div className="emaildetail__heading">
          <h2 id={titleId} className="emaildetail__title">
            {title}
          </h2>
          {/* WHAT this message's access is, beside its subject. A limit is a
              fact about a message like its date, and a reader wants it before
              they read rather than after: under the body these markers sat
              below the attachments, so on anything longer than a screen the
              first sign a message was confidential arrived once the reader had
              already finished it.

              Drawn from the read the drawer has already made, so a header with
              no markers is a message whose access block did not arrive — never
              one whose access nobody asked about. */}
          {read.data && renderAccessMarkers?.(read.data.access)}
        </div>
        <Button
          small
          iconOnly
          onClick={onClose}
          aria-label={t("email.detail.close")}
        >
          <X aria-hidden="true" />
        </Button>
      </div>
      {read.isPending ? (
        <SurfaceState
          state="loading"
          emptyLabel={t("email.detail.none")}
          loadingLabel={t("email.detail.loading")}
        >
          {null}
        </SurfaceState>
      ) : read.isError || !read.data ? (
        // `failed` with a retry rather than `unavailable`: the read can be
        // asked again, and a failure with nothing to press is the same as
        // being told the message is not there.
        <SurfaceState
          loadingLabel={t("email.detail.loading")}
          state="failed"
          emptyLabel={t("email.detail.none")}
          detail={{ onRetry: () => void read.refetch() }}
        >
          {null}
        </SurfaceState>
      ) : (
        <EmailBody
          presentation={read.data}
          formatWhen={formatWhen}
          renderAccess={renderAccess}
        />
      )}
    </Modal>
  );
}

function EmailBody({
  presentation,
  formatWhen,
  renderAccess,
}: Readonly<{
  presentation: EmailPresentation;
  formatWhen: (iso: string) => string;
  renderAccess?: (presentation: EmailPresentation) => ReactNode;
}>) {
  const t = useT();
  if (presentation.access.content_state === "withheld") {
    // The row stays, its words do not, and the reason does not either: why a
    // message is limited describes what it is about.
    return (
      <SurfaceState
        loadingLabel={t("email.detail.loading")}
        state="withheld"
        emptyLabel={t("email.detail.none")}
        // The generic sentence says "your role cannot read this", and an
        // audience is not a role: the author of one message limited it, which
        // says nothing about the seat this reader holds. Naming the real
        // reason would describe the message, so this names neither.
        detail={{ withheldReason: t("email.detail.withheldReason") }}
      >
        {null}
      </SurfaceState>
    );
  }
  const parts = splitEmailBody(presentation.body ?? "");
  return (
    <div className="emaildetail__body">
      <Parties presentation={presentation} formatWhen={formatWhen} />
      <p className="emaildetail__main">{parts.main}</p>
      {/* A SIGN-OFF is the sender still speaking, and it is two lines. It is
          shown, quietly, under the message it belongs to.

          Folding it away was the defect: the tail was one field for two
          different things, so a message ending "Viele Grüße / Bảo" and no
          quoted reply at all put the sender's own name behind a control
          promising history that was not there. A reader pressed nothing,
          because the label said the thing they did not want. */}
      {parts.tail === "signature" && (
        <p className="emaildetail__signoff">{parts.trimmed}</p>
      )}
      {/* An older message under this one. Kept and folded rather than dropped:
          a splitter that guesses wrong must stay one press from being wrong in
          public. */}
      {parts.tail === "quote" && (
        <details className="emaildetail__quoted">
          <summary>{t("email.detail.showQuoted")}</summary>
          <p>{parts.trimmed}</p>
        </details>
      )}
      <Attachments files={presentation.attachments} />
      {/* Who reads this, last: a reader came for the message, and the limit on
          it is what they check after reading rather than before. The withheld
          branch above returns before here on purpose — that reader is told the
          message is not shared with them, which is the whole of what the
          access block would say, and `can_change` is false for them anyway. */}
      {renderAccess?.(presentation)}
    </div>
  );
}

/**
 * What came with the message.
 *
 * Reached only from the body above, which returns early for a withheld
 * message — so a reader outside the audience is never told that a contract
 * arrived, which is a fact about the message like any other.
 *
 * The NAME is the download, the pattern the account and contact file lists
 * already use: a separate action word at the far end of the row is a second
 * thing to find for the only thing this row does. The href is the attachment
 * endpoint that has always served these bytes; nothing new is fetched here.
 */
function Attachments({ files }: Readonly<{ files: EmailAttachmentSummary[] }>) {
  const { locale } = useLocale();
  if (files.length === 0) {
    // No empty region: a heading over nothing says the message had files and
    // they are missing, which is a different claim from having had none.
    return null;
  }
  return (
    <div className="emaildetail__files">
      <p className="emaildetail__filesLabel">
        {translatePlural(locale, "email.detail.attachments", files.length, {
          count: formatNumber(files.length, locale),
        })}
      </p>
      <ul className="emaildetail__fileList">
        {files.map((file) => (
          <li key={file.id} className="emaildetail__file">
            <Paperclip aria-hidden="true" />
            <a
              className="link-button"
              href={`/v1/attachments/${file.id}`}
              download={file.filename}
            >
              {file.filename}
            </a>
            {/* Absent rather than zero when the server sent no size: a size it
                could not record is not a file of no bytes. */}
            {file.byte_size != null && (
              <span className="emaildetail__fileSize">
                {formatBytes(file.byte_size, locale)}
              </span>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * partyName is what one participant is called on a header line.
 *
 * The name, then the address, then nothing — and the LAST step is the one that
 * matters. `display_name ?? address` renders an empty string as empty, because
 * `??` only catches null: a party with neither produced a bare comma in the
 * middle of the To line, which reads as a recipient whose name we lost rather
 * than as a row the response never filled in.
 */
function partyName(party: EmailParty): string {
  return party.display_name?.trim() || party.address.trim();
}

function PartyLine({
  label,
  parties,
}: Readonly<{ label: string; parties: EmailParty[] }>) {
  // Only the parties that can actually be named. A row carrying neither a name
  // nor an address says nothing to a reader, and joining it in puts a gap in
  // the list where a person should be — so it is dropped, and a line with
  // nobody left to name does not draw at all.
  const named = parties.map(partyName).filter(Boolean);
  if (named.length === 0) {
    return null;
  }
  return (
    <p className="emaildetail__party">
      <span className="emaildetail__partyLabel">{label}</span>
      {named.join(", ")}
    </p>
  );
}

function Parties({
  presentation,
  formatWhen,
}: Readonly<{
  presentation: EmailPresentation;
  formatWhen: (iso: string) => string;
}>) {
  const t = useT();
  return (
    <div className="emaildetail__parties">
      <PartyLine label={t("email.detail.from")} parties={presentation.from} />
      <PartyLine label={t("email.detail.to")} parties={presentation.to} />
      <PartyLine label={t("email.detail.cc")} parties={presentation.cc} />
      {/* WHEN it was sent, beside who it was sent to. It used to sit under the
          message, below the attachments — so on anything longer than a screen
          the reader had to scroll past the whole body to learn the date, which
          is one of the first things they came for. It is an envelope fact and
          it belongs with the others. */}
      <p className="emaildetail__party">
        <span className="emaildetail__partyLabel">
          {t("email.detail.when")}
        </span>
        {formatWhen(presentation.occurred_at)}
      </p>
      {/* Said rather than shown: an absent BCC list reads as "nobody was
          blind-copied", which is a different fact from "you may not see who
          was". Only the sending seat gets the names. */}
      {presentation.bcc_withheld && (
        <p className="emaildetail__party emaildetail__party--withheld">
          {t("email.detail.bccWithheld")}
        </p>
      )}
    </div>
  );
}
