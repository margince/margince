// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { splitEmailBody } from "../format/emailtext";
import { useT } from "../i18n";
import { Modal } from "./atoms";
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
  const read = useQuery({
    queryKey: emailDetailKey(activityId),
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

  const title = read.data
    ? read.data.summary.subject?.trim() ||
      (read.data.access.content_state === "withheld"
        ? t("email.withheldSubject")
        : t("email.noSubject"))
    : t("email.detail.none");

  return (
    <Modal
      open
      onClose={onClose}
      labelledBy="emaildetail-title"
      placement="right"
      size="wide"
    >
      <h2 id="emaildetail-title" className="emaildetail__title">
        {title}
      </h2>
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
      <SurfaceState state="withheld" emptyLabel={t("email.detail.none")}>
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
      <p className="emaildetail__when">{formatWhen(presentation.occurred_at)}</p>
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
