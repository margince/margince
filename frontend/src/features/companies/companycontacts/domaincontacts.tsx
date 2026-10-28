// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useId, useState, useSyncExternalStore } from "react";
import { api } from "../../../api/client";
import { useIdempotencyKey } from "../../../api/idempotencykey";
import type { components } from "../../../api/schema";
import {
  Button,
  Checkbox,
  EmptyState,
  Modal,
  PendingBody,
} from "../../../design-system/atoms";
import { Callout } from "../../../design-system/callout";
import { ErrorLine } from "../../../design-system/errorline";
import { Heading } from "../../../design-system/heading";
import { useToast } from "../../../design-system/toast";
import { usePlural, useT } from "../../../i18n";
import { useCanWrite, useGrantRefused } from "../../../lib/capability";
import {
  problemFieldErrorsOf,
  problemStatus,
  throwProblem,
  unwrap,
} from "../../../lib/problem";
import {
  invalidateCompany,
  invalidateRecord,
} from "../../../lib/recordwritekeys";
import { companyContactsListKey } from "./contacts.keys";
import "./companycontacts.css";

type CandidateList = components["schemas"]["CompanyLinkCandidateList"];
type Candidate = components["schemas"]["CompanyLinkCandidate"];

function linkCandidatesKey(companyId: string) {
  return ["company-link-candidates", companyId] as const;
}

function useLinkCandidates(companyId: string, enabled: boolean) {
  return useQuery({
    queryKey: linkCandidatesKey(companyId),
    enabled,
    queryFn: async (): Promise<CandidateList> => {
      const { data, error } = await api.GET("/companies/{id}/link-candidates", {
        params: { path: { id: companyId } },
      });
      if (error || !data) {
        return throwProblem(error);
      }
      return data;
    },
  });
}

// Linking needs the edge grant and a write on each contact; the server refuses
// the whole batch when either is missing.
function useMayLink(): boolean {
  const edge = useCanWrite("relationship", "create");
  const contact = useCanWrite("contact", "update");
  return edge && contact;
}

function useLinkRefused(): boolean {
  const edge = useGrantRefused({ object: "relationship", action: "create" });
  const contact = useGrantRefused({ object: "contact", action: "update" });
  return edge || contact;
}

function isNotACandidate(error: unknown): boolean {
  return problemFieldErrorsOf(error).some(
    (field) =>
      field.field === "contact_ids" && field.code === "not_a_link_candidate",
  );
}

async function refreshAfterLink(
  client: QueryClient,
  companyId: string,
  contactIds: readonly string[],
): Promise<void> {
  await Promise.all([
    invalidateCompany(client, { id: companyId }),
    client.invalidateQueries({ queryKey: linkCandidatesKey(companyId) }),
    client.invalidateQueries({
      queryKey: [companyContactsListKey(companyId)],
    }),
    client.invalidateQueries({ queryKey: ["company-coverage", companyId] }),
    client.invalidateQueries({ queryKey: ["companies"] }),
    ...contactIds.map((id) => invalidateRecord(client, "contact", id)),
  ]);
}

// Nothing starts ticked: a shared address domain is a hint, not a fact.
export function DomainContactsDialog({
  companyId,
  open,
  onClose,
}: Readonly<{ companyId: string; open: boolean; onClose: () => void }>) {
  const t = useT();
  const plural = usePlural();
  const toast = useToast();
  const queryClient = useQueryClient();
  const headingId = useId();
  const candidates = useLinkCandidates(companyId, open);
  const [ticked, setTicked] = useState<ReadonlySet<string>>(new Set());
  const { key, renew } = useIdempotencyKey();
  const link = useMutation({
    mutationFn: async ({
      contactIds,
      idempotencyKey,
    }: {
      contactIds: string[];
      idempotencyKey: string;
    }) =>
      unwrap(
        await api.POST("/companies/{id}/link-contacts", {
          params: {
            path: { id: companyId },
            header: { "Idempotency-Key": idempotencyKey },
          },
          body: { contact_ids: contactIds },
        }),
        t,
      ),
    onSuccess: async (_linked, { contactIds }) => {
      renew();
      toast.show(
        plural("co.domainContacts.linked", contactIds.length, {
          count: String(contactIds.length),
        }),
      );
      setTicked(new Set());
      onClose();
      await refreshAfterLink(queryClient, companyId, contactIds);
    },
    onError: async (error) => {
      if (isNotACandidate(error)) {
        renew();
        await candidates.refetch();
      }
    },
  });
  const rows = candidates.data?.data ?? [];
  const chosen = rows
    .filter((candidate) => ticked.has(candidate.contact_id))
    .map((candidate) => candidate.contact_id);
  const changed = isNotACandidate(link.error);
  const close = () => {
    setTicked(new Set());
    link.reset();
    onClose();
  };
  const toggle = (id: string, on: boolean) => {
    const next = new Set(ticked);
    if (on) {
      next.add(id);
    } else {
      next.delete(id);
    }
    setTicked(next);
  };

  return (
    <Modal
      open={open}
      onClose={close}
      labelledBy={headingId}
      intent="form"
      closeDisabled={link.isPending}
    >
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("co.domainContacts.dialogTitle")}
      </Heading>
      <div className="form-stack">
        {changed && (
          <Callout
            tone="warning"
            kind="outcome"
            title={t("co.domainContacts.changed")}
          />
        )}
        <CandidateRows
          query={candidates}
          rows={rows}
          ticked={ticked}
          onToggle={toggle}
        />
        {!changed && <ErrorLine error={link.error} />}
      </div>
      <div className="actions">
        <Button onClick={close} disabled={link.isPending}>
          {t("co.domainContacts.dismiss")}
        </Button>
        <Button
          variant="primary"
          disabled={chosen.length === 0}
          pending={link.isPending}
          onClick={() =>
            link.mutate({ contactIds: chosen, idempotencyKey: key })
          }
        >
          {chosen.length === 0
            ? t("co.domainContacts.linkNone")
            : plural("co.domainContacts.link", chosen.length, {
                count: String(chosen.length),
              })}
        </Button>
      </div>
    </Modal>
  );
}

function CandidateRows({
  query,
  rows,
  ticked,
  onToggle,
}: Readonly<{
  query: ReturnType<typeof useLinkCandidates>;
  rows: readonly Candidate[];
  ticked: ReadonlySet<string>;
  onToggle: (id: string, on: boolean) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  if (query.isError) {
    return <ErrorLine error={query.error} />;
  }
  if (!query.data) {
    return <PendingBody label={t("co.domainContacts.loading")} />;
  }
  if (rows.length === 0) {
    return <EmptyState>{t("co.domainContacts.none")}</EmptyState>;
  }
  return (
    <>
      <p>{t("co.domainContacts.intro")}</p>
      {rows.map((candidate) => (
        <Checkbox
          key={candidate.contact_id}
          className="cp-candidate"
          checked={ticked.has(candidate.contact_id)}
          onChange={(event) =>
            onToggle(candidate.contact_id, event.currentTarget.checked)
          }
          label={
            <>
              <span className="cp-candidate-line">
                <strong>{candidate.full_name}</strong>
                {candidate.title && (
                  <span className="t-caption"> · {candidate.title}</span>
                )}
              </span>
              <span className="cp-candidate-line t-caption">
                {candidate.email}
              </span>
              {candidate.employer && (
                <span className="cp-candidate-line t-caption">
                  {t("co.domainContacts.worksAt", {
                    company: candidate.employer.company_name,
                  })}
                </span>
              )}
            </>
          }
        />
      ))}
      {query.data.truncated && (
        <Callout
          kind="standing"
          title={plural("co.domainContacts.truncated", rows.length, {
            count: String(rows.length),
          })}
        />
      )}
    </>
  );
}

// A reader the candidate read refuses sees nothing, not an error.
export function DomainContactsCallout({
  companyId,
}: Readonly<{ companyId: string }>) {
  const t = useT();
  const plural = usePlural();
  const refused = useLinkRefused();
  const [open, setOpen] = useState(false);
  const candidates = useLinkCandidates(companyId, true);
  const list = candidates.data;
  let notice = null;
  if (candidates.isError && problemStatus(candidates.error) !== 403) {
    notice = <ErrorLine error={candidates.error} standing />;
  } else if (list && list.data.length > 0) {
    const count = String(list.data.length);
    notice = (
      <Callout
        kind="standing"
        title={
          list.truncated
            ? plural("co.domainContacts.calloutMore", list.data.length, {
                count,
              })
            : plural("co.domainContacts.callout", list.data.length, { count })
        }
        actions={
          <Button
            reason={refused ? t("co.domainContacts.denied") : undefined}
            onClick={() => setOpen(true)}
          >
            {t("co.domainContacts.review")}
          </Button>
        }
      />
    );
  }
  return (
    <>
      {notice}
      <DomainContactsDialog
        companyId={companyId}
        open={open}
        onClose={() => setOpen(false)}
      />
    </>
  );
}

// A company created from the list has its domain contacts offered once, on
// the record page the create opens next.
const offers = new Set<string>();
const offerListeners = new Set<() => void>();

function announceOffers() {
  for (const listener of offerListeners) listener();
}

function subscribeOffers(listener: () => void) {
  offerListeners.add(listener);
  return () => {
    offerListeners.delete(listener);
  };
}

function settleOffer(companyId: string) {
  if (offers.delete(companyId)) announceOffers();
}

export function offerDomainContacts(
  company: Readonly<{ id: string; domains?: readonly unknown[] }>,
): void {
  if ((company.domains ?? []).length === 0) return;
  offers.add(company.id);
  announceOffers();
}

export function DomainContactsArrival({
  companyId,
}: Readonly<{ companyId: string }>) {
  const offered = useSyncExternalStore(subscribeOffers, () =>
    offers.has(companyId),
  );
  const mayLink = useMayLink();
  const candidates = useLinkCandidates(companyId, offered && mayLink);
  const [shown, setShown] = useState(false);
  const found = (candidates.data?.data.length ?? 0) > 0;
  if (offered && mayLink && found && !shown) {
    setShown(true);
  }
  const nothingToOffer =
    candidates.isError || candidates.data?.data.length === 0;
  useEffect(() => {
    if (offered && nothingToOffer && !shown) settleOffer(companyId);
  }, [offered, nothingToOffer, shown, companyId]);
  return (
    <DomainContactsDialog
      companyId={companyId}
      open={shown}
      onClose={() => {
        setShown(false);
        settleOffer(companyId);
      }}
    />
  );
}
