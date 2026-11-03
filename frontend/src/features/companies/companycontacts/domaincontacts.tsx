// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  useCallback,
  useEffect,
  useId,
  useState,
  useSyncExternalStore,
} from "react";
import { api } from "../../../api/client";
import { useIdempotencyKey } from "../../../api/idempotencykey";
import type { components } from "../../../api/schema";
import {
  Button,
  EmptyState,
  Modal,
  PendingBody,
} from "../../../design-system/atoms";
import { Callout } from "../../../design-system/callout";
import { ChoiceList } from "../../../design-system/choicelist";
import { ErrorLine } from "../../../design-system/errorline";
import { Heading } from "../../../design-system/heading";
import { useToast } from "../../../design-system/toast";
import { formatNumber } from "../../../format/format";
import { useLocale, usePlural, useT } from "../../../i18n";
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
  const { locale } = useLocale();
  const toast = useToast();
  const queryClient = useQueryClient();
  const headingId = useId();
  const candidates = useLinkCandidates(companyId, open);
  const [ticked, setTicked] = useState<readonly string[]>([]);
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
          count: formatNumber(contactIds.length, locale),
        }),
      );
      setTicked([]);
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
    .filter((candidate) => ticked.includes(candidate.contact_id))
    .map((candidate) => candidate.contact_id);
  const changed = isNotACandidate(link.error);
  const close = () => {
    setTicked([]);
    link.reset();
    onClose();
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
          ticked={chosen}
          onTick={(ids) => {
            setTicked(ids);
            renew();
          }}
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
                count: formatNumber(chosen.length, locale),
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
  onTick,
}: Readonly<{
  query: ReturnType<typeof useLinkCandidates>;
  rows: readonly Candidate[];
  ticked: readonly string[];
  onTick: (ids: string[]) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
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
      <ChoiceList
        legend={t("co.domainContacts.legend")}
        hideLegend
        multiple
        value={ticked}
        onChange={onTick}
        choices={rows.map((candidate) => ({
          value: candidate.contact_id,
          label: candidate.full_name,
          description: <CandidateFacts candidate={candidate} />,
        }))}
      />
      {query.data.truncated && (
        <Callout
          kind="standing"
          title={plural("co.domainContacts.truncated", rows.length, {
            count: formatNumber(rows.length, locale),
          })}
        />
      )}
    </>
  );
}

function CandidateFacts({ candidate }: Readonly<{ candidate: Candidate }>) {
  const t = useT();
  return (
    <span className="cp-candidate t-caption">
      {candidate.title && <span>{candidate.title}</span>}
      <span>{candidate.email}</span>
      {candidate.employer && (
        <span>
          {t("co.domainContacts.worksAt", {
            company: candidate.employer.company_name,
          })}
        </span>
      )}
    </span>
  );
}

// A reader the candidate read refuses sees nothing, not an error.
export function DomainContactsCallout({
  companyId,
}: Readonly<{ companyId: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const candidates = useLinkCandidates(companyId, true);
  const list = candidates.data;
  let notice = null;
  if (candidates.isError && problemStatus(candidates.error) !== 403) {
    notice = <ErrorLine error={candidates.error} standing />;
  } else if (list && list.data.length > 0) {
    const count = formatNumber(list.data.length, locale);
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
          <Button onClick={() => setOpen(true)}>
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

// A company created from the list has its domain contacts offered once, on the
// record page the create opens next. The offer is kept in the query cache, so
// a sign-out, which empties the cache, withdraws it.
function offerKey(companyId: string) {
  return ["domain-contacts-offer", companyId] as const;
}

function useOffered(client: QueryClient, companyId: string): boolean {
  const subscribe = useCallback(
    (listener: () => void) => client.getQueryCache().subscribe(listener),
    [client],
  );
  return useSyncExternalStore(
    subscribe,
    () => client.getQueryData(offerKey(companyId)) === true,
  );
}

function settleOffer(client: QueryClient, companyId: string) {
  client.removeQueries({ queryKey: offerKey(companyId), exact: true });
}

export function offerDomainContacts(
  client: QueryClient,
  company: Readonly<{ id: string; domains?: readonly unknown[] }>,
): void {
  if ((company.domains ?? []).length === 0) return;
  client.setQueryData(offerKey(company.id), true);
}

export function DomainContactsArrival({
  companyId,
}: Readonly<{ companyId: string }>) {
  const client = useQueryClient();
  const offered = useOffered(client, companyId);
  // A role that may not link reads an empty list, which spends the offer.
  const candidates = useLinkCandidates(companyId, offered);
  const [shown, setShown] = useState(false);
  const found = (candidates.data?.data.length ?? 0) > 0;
  if (offered && found && !shown) {
    setShown(true);
  }
  const nothingToOffer =
    candidates.isError || candidates.data?.data.length === 0;
  useEffect(() => {
    if (offered && nothingToOffer && !shown) settleOffer(client, companyId);
  }, [offered, nothingToOffer, shown, client, companyId]);
  // Leaving the page with the dialog open spends the offer, as closing it does.
  useEffect(() => {
    if (!shown) return undefined;
    return () => settleOffer(client, companyId);
  }, [shown, client, companyId]);
  return (
    <DomainContactsDialog
      companyId={companyId}
      open={shown}
      onClose={() => {
        settleOffer(client, companyId);
        setShown(false);
      }}
    />
  );
}
