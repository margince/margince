// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite, useCanWriteRecord } from "../app/capability";
import { Button, Checkbox, Modal, PendingBody } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import {
  BulkChangeDialog,
  type BulkChangeRequest,
  type BulkRow,
} from "./bulkchange";
import type { PickedTag } from "./bulktag";
import { throwProblem } from "./common";
import { useEmployerSummary } from "./contactemployersummary";
import { useApplyTag, useRecordTags } from "./tags.queries";

// A tag on a contact and the same tag on their company are separate facts, and
// users who tag one usually want the other. These offers ask once, right after
// the apply; nothing is written until the reader presses.

type ContactEmployer = components["schemas"]["ContactEmployer"];
type Contact = components["schemas"]["Contact"];

/** The bulk API takes at most this many items, so the roster reads one page of it. */
const ROSTER_LIMIT = 200;

/**
 * "Also tag <company>?" after a contact was tagged. Silent when the reader may
 * not change the company, when its tags are hidden from them, or when it
 * already carries the word.
 */
export function EmployerTagOffer({
  employer,
  tag,
  onClose,
}: Readonly<{
  employer: ContactEmployer;
  tag: PickedTag;
  onClose: () => void;
}>) {
  const t = useT();
  const toast = useToast();
  const company = useEmployerSummary(employer.company_id);
  const canWrite = useCanWriteRecord("company", company.data ?? undefined);
  const tags = useRecordTags("company", employer.company_id);
  const apply = useApplyTag("company", employer.company_id);
  const carried = (tags.data?.data ?? []).some((on) => on.tag_id === tag.id);
  if (
    !canWrite ||
    company.data?.archived_at ||
    tags.data === undefined ||
    tags.data.withheld ||
    carried
  ) {
    return null;
  }
  const words = { company: employer.company_name, tag: tag.name };
  return (
    <Callout
      title={t("tags.offerCompanyTitle", words)}
      dismiss={{ label: t("tags.offerDismiss"), onDismiss: onClose }}
      actions={
        <Button
          variant="primary"
          pending={apply.isPending}
          onClick={() =>
            apply.mutate(tag.id, {
              onSuccess: () => {
                toast.show(t("tags.offerCompanyDone", words));
                onClose();
              },
            })
          }
        >
          {t("tags.offerCompanyAccept", words)}
        </Button>
      }
    >
      <ErrorLine error={apply.error} />
    </Callout>
  );
}

/**
 * "Also tag contacts at this company?" after a company was tagged. The press
 * opens a list of the company's current contacts to tick. The change runs as a
 * bulk change: a contact the reader may not edit is named, and Undo reverts it.
 */
export function ContactsTagOffer({
  companyID,
  tag,
  onClose,
}: Readonly<{ companyID: string; tag: PickedTag; onClose: () => void }>) {
  const t = useT();
  const [choosing, setChoosing] = useState(false);
  const [request, setRequest] = useState<BulkChangeRequest | null>(null);
  // Tagging a contact writes to the contact. A reader without contact.update
  // would only meet the bulk change's refusal, so the offer is not made.
  const mayTagContacts = useCanWrite("contact", "update");
  if (!mayTagContacts) {
    return null;
  }
  return (
    <>
      <Callout
        title={t("tags.offerContactsTitle", { tag: tag.name })}
        dismiss={{ label: t("tags.offerDismiss"), onDismiss: onClose }}
        actions={
          <Button variant="primary" onClick={() => setChoosing(true)}>
            {t("tags.offerContactsAccept")}
          </Button>
        }
      />
      <ChooseContactsDialog
        open={choosing}
        companyID={companyID}
        tag={tag}
        onClose={() => setChoosing(false)}
        onChosen={(rows) => {
          setChoosing(false);
          setRequest({
            recordType: "contact",
            verb: "add_tag",
            rows,
            tag,
            openId: crypto.randomUUID(),
          });
        }}
      />
      <BulkChangeDialog
        request={request}
        onClose={() => setRequest(null)}
        onDone={() => {
          setRequest(null);
          onClose();
        }}
      />
    </>
  );
}

function useCompanyRoster(companyID: string, enabled: boolean) {
  return useQuery({
    queryKey: ["contacts", "employed-at", companyID],
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/contacts", {
        params: { query: { company_id: companyID, limit: ROSTER_LIMIT } },
      });
      if (error || !data) {
        return throwProblem(error);
      }
      return { contacts: data.data, truncated: data.page.has_more };
    },
  });
}

function ChooseContactsDialog({
  open,
  companyID,
  tag,
  onClose,
  onChosen,
}: Readonly<{
  open: boolean;
  companyID: string;
  tag: PickedTag;
  onClose: () => void;
  onChosen: (rows: BulkRow[]) => void;
}>) {
  const t = useT();
  const headingId = useId();
  const roster = useCompanyRoster(companyID, open);
  const [ticked, setTicked] = useState<ReadonlySet<string>>(new Set());
  const contacts = roster.data?.contacts ?? [];
  const carries = (contact: Contact) =>
    (contact.tags ?? []).some((on) => on.tag_id === tag.id);
  const toggle = (id: string, on: boolean) => {
    const next = new Set(ticked);
    if (on) {
      next.add(id);
    } else {
      next.delete(id);
    }
    setTicked(next);
  };
  const chosen = contacts.filter((contact) => ticked.has(contact.id));

  let body = <PendingBody label={t("tags.contactsLoading")} />;
  if (roster.isError) {
    body = <ErrorLine error={roster.error} />;
  } else if (roster.data && contacts.length === 0) {
    body = <p>{t("tags.contactsNone")}</p>;
  } else if (roster.data) {
    body = (
      <>
        {contacts.map((contact) => {
          const already = carries(contact);
          return (
            <Checkbox
              key={contact.id}
              label={
                already
                  ? t("tags.contactsAlready", { name: contact.full_name })
                  : contact.full_name
              }
              checked={already || ticked.has(contact.id)}
              disabled={already}
              onChange={(event) =>
                toggle(contact.id, event.currentTarget.checked)
              }
            />
          );
        })}
        {roster.data.truncated && (
          <Callout kind="standing" title={t("tags.contactsTruncated")} />
        )}
      </>
    );
  }

  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("tags.contactsTitle", { tag: tag.name })}
      </Heading>
      <div className="form-stack">{body}</div>
      <div className="actions">
        <Button onClick={onClose}>{t("create.cancel")}</Button>
        <Button
          variant="primary"
          disabled={chosen.length === 0}
          onClick={() =>
            onChosen(
              chosen.map((contact) => ({
                id: contact.id,
                version: contact.version,
                label: contact.full_name,
              })),
            )
          }
        >
          {t("tags.contactsContinue")}
        </Button>
      </div>
    </Modal>
  );
}
