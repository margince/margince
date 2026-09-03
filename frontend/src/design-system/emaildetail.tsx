// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { X } from "lucide-react";
import { useId } from "react";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { splitEmailBody } from "../format/emailtext";
import { useT } from "../i18n";
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
}: Readonly<{
  activityId: string;
  onClose: () => void;
  /** The caller owns the reader's timezone, so it owns the formatting. */
  formatWhen: (iso: string) => string;
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
        <h2 id={titleId} className="emaildetail__title">
          {title}
        </h2>
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
          state="failed"
          emptyLabel={t("email.detail.none")}
          detail={{ onRetry: () => void read.refetch() }}
        >
          {null}
        </SurfaceState>
      ) : (
        <EmailBody presentation={read.data} formatWhen={formatWhen} />
      )}
    </Modal>
  );
}

function EmailBody({
  presentation,
  formatWhen,
}: Readonly<{
  presentation: EmailPresentation;
  formatWhen: (iso: string) => string;
}>) {
  const t = useT();
  if (presentation.access.content_state === "withheld") {
    // The row stays, its words do not, and the reason does not either: why a
    // message is limited describes what it is about.
    return (
      <SurfaceState
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
      <Parties presentation={presentation} />
      <p className="emaildetail__main">{parts.main}</p>
      {/* Kept and folded rather than dropped: a splitter that guesses wrong
          must stay one press from being wrong in public. */}
      {parts.trimmed && (
        <details className="emaildetail__quoted">
          <summary>{t("email.detail.showQuoted")}</summary>
          <p>{parts.trimmed}</p>
        </details>
      )}
      <p className="emaildetail__when">
        {formatWhen(presentation.occurred_at)}
      </p>
    </div>
  );
}

function PartyLine({
  label,
  parties,
}: Readonly<{ label: string; parties: EmailParty[] }>) {
  if (parties.length === 0) {
    return null;
  }
  return (
    <p className="emaildetail__party">
      <span className="emaildetail__partyLabel">{label}</span>
      {parties.map((p) => p.display_name ?? p.address).join(", ")}
    </p>
  );
}

function Parties({
  presentation,
}: Readonly<{ presentation: EmailPresentation }>) {
  const t = useT();
  return (
    <div className="emaildetail__parties">
      <PartyLine label={t("email.detail.from")} parties={presentation.from} />
      <PartyLine label={t("email.detail.to")} parties={presentation.to} />
      <PartyLine label={t("email.detail.cc")} parties={presentation.cc} />
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
