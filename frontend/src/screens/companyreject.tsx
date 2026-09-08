// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useCanWrite } from "../app/capability";
import { Button, Field, Modal } from "../design-system/atoms";
import { useT } from "../i18n";
import { useArchiveRecord } from "./archive";
import { problemMessageOf, throwProblem } from "./common";

type Organization = components["schemas"]["Organization"];

// "This is not a company": one press, one request, both halves.
//
// It is a separate verb from Archive because archiving alone does not settle
// it. The record came from mail, so the next message on the same domain mints
// it again and the person who archived it learns nothing about why it is back.
//
// One call, deliberately. The first attempt at this was two — block the domain,
// then archive — and it could not be atomic: a rep who held the update grant
// and not delete blocked the domain, took a 403 on the archive, and the screen,
// which surfaced only the last error, told them nothing had happened. The
// server does both in one transaction now and this asks for it once.
export function CompanyRejectAction({
  org,
  disabledReasonId,
}: Readonly<{
  org: Organization;
  // Why this account's verbs are refused, when they are — the archived-record
  // sentence the whole menu shares. See CompanyActionBadges.
  disabledReasonId?: string;
}>) {
  const t = useT();
  const headingId = useId();
  const [confirming, setConfirming] = useState(false);
  const [reason, setReason] = useState("");
  // BOTH grants, because the write needs both and asks for both up front: the
  // archive is `organization:delete` and the standing domain decision is
  // `organization:update`. Offering this to a seat holding one of them is what
  // put a suppressed domain behind a company that was still there.
  const mayArchive = useCanWrite("organization", "delete");
  const mayDecideDomains = useCanWrite("organization", "update");
  // The action exists only where there is a domain to refuse. A company
  // somebody typed in by hand was never derived from mail, so no refusal would
  // stop anything, and the server says so — but a control that is only ever
  // refused is a control that should not be drawn.
  //
  // The value shown below is what the PAGE holds; the server reads the primary
  // again inside the transaction, and the message on the way out names the one
  // it actually refused.
  const primary = (org.domains ?? []).find((d) => d.is_primary);
  const mutation = useArchiveRecord({
    archive: async () => {
      const { data, error } = await api.POST("/organizations/{id}/reject", {
        params: {
          path: { id: org.id },
          ...ifMatch(requireVersion(org.version)),
        },
        body: { reason: reason.trim() },
      });
      if (error) {
        throwProblem(error, t);
      }
      return { id: data.organization.id, domain: data.domain.domain };
    },
    invalidate: "organizations",
    recordKey: "organization",
    archivedMessage: ({ domain }) =>
      t("org.rejectDone", { name: org.display_name, domain }),
    onDone: () => {
      setConfirming(false);
      setReason("");
    },
  });

  // Absent rather than disabled: STATE-4a sorts by CAUSE, and a control the
  // reader has no authority for — or one with nothing to act on — reports no
  // fact about this account worth showing them.
  if (!mayArchive || !mayDecideDomains || !primary) {
    return null;
  }
  const ready = reason.trim().length > 0;
  return (
    <>
      <Button
        small
        variant="danger"
        reasonId={disabledReasonId}
        onClick={() => setConfirming(true)}
        data-testid="reject-company"
      >
        {t("org.reject")}
      </Button>
      <Modal
        open={confirming}
        onClose={() => setConfirming(false)}
        labelledBy={headingId}
      >
        <h2
          id={headingId}
          className="t-h2"
          style={{ marginBottom: "var(--space-3)" }}
        >
          {t("org.reject")}
        </h2>
        <p style={{ marginBottom: "var(--space-4)" }}>
          {t("org.rejectConfirm", {
            name: org.display_name,
            domain: primary.domain,
          })}
        </p>
        <Field
          label={t("org.rejectReasonLabel")}
          hint={t("org.rejectReasonHint")}
          required
        >
          {(control) => (
            <textarea
              {...control}
              rows={3}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
          )}
        </Field>
        {mutation.isError && (
          // role="alert" so a refused rejection is announced: the dialog stays
          // open either way, and without this the only difference between "it
          // failed" and "it is still working" is a line of red text.
          <p
            className="t-caption"
            role="alert"
            style={{ color: "var(--danger)" }}
          >
            {problemMessageOf(mutation.error, t)}
          </p>
        )}
        <div className="actions">
          <Button
            small
            onClick={() => setConfirming(false)}
            disabled={mutation.isPending}
          >
            {t("create.cancel")}
          </Button>
          <Button
            small
            variant="danger"
            // The reason is REQUIRED by the contract, so the control that
            // sends it says so before the server has to: a refusal for an
            // empty field the reader can see is a round trip that teaches
            // them nothing.
            disabled={!ready || mutation.isPending}
            onClick={() => mutation.mutate()}
            data-testid="reject-company-confirm"
          >
            {t("org.reject")}
          </Button>
        </div>
      </Modal>
    </>
  );
}
