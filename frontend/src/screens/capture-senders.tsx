import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { RowOpen } from "../design-system/rowopen";
import { useToast } from "../design-system/toast";
import { formatDate, formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem, unwrap } from "../lib/problem";
import { navigate } from "../lib/router";
import { EntityRef } from "./entityref";
import "./capture-senders.css";

// What the classifier decided about each sender this mailbox brought in, and
// the seat's own answer where they gave one.
//
// The page exists because the posture rests on being checkable. A product that
// decides silently which of your correspondents becomes a contact — and which
// of them is family, and whose mail gets destroyed — is one a contact has to
// trust rather than audit. This is the audit.
//
// Read-only for everybody but the owner: the endpoint answers the caller's own
// senders and has no admin view, because whose mail a colleague keeps out is
// itself private.

type SenderDecision = components["schemas"]["CaptureSenderDecision"];
type AwaitingContact = components["schemas"]["CaptureAwaitingContact"];

// The classifier's vocabulary, in the reader's words. A kind absent from the
// map is one the server learned to say and this screen has not.
const kindLabel: Record<string, MessageKey> = {
  contact: "senders.kind.contact",
  role_mailbox: "senders.kind.roleMailbox",
  company_sender: "senders.kind.companySender",
  newsletter: "senders.kind.newsletter",
  transactional: "senders.kind.transactional",
  spam: "senders.kind.spam",
  personal: "senders.kind.personal",
  advisor: "senders.kind.advisor",
};

// Which kinds mean "this sender's mail is in the CRM as a contact". The tone
// carries it at a glance down the column; the words still say it, because
// colour is never the only signal.
const admitted = new Set(["contact", "role_mailbox", "company_sender"]);

function useSenders() {
  return useQuery({
    queryKey: ["capture-senders"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/capture/senders");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// The contacts this seat's mail created that stay visible to them alone until
// a sender decision is reached — the set the admin capture-health card counts
// for this mailbox. Its own read: a contact whose address never reached the
// senders list above is waiting too, and that list cannot show it.
function useAwaitingDecision() {
  return useQuery({
    queryKey: ["capture-contacts-awaiting-decision"],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/capture/contacts-awaiting-decision",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function useSetDecision() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    // Address and decision both arrive as variables: the row a reader pressed
    // belongs to the committed render, and a mutationFn reading render state
    // would answer with the previous one (frontend/AGENTS.md,
    // mutation-variable-coverage).
    mutationFn: async (vars: {
      address: string;
      decision: "business" | "keep_out";
    }) => {
      unwrap(
        await api.PUT("/capture/senders/{address}/decision", {
          params: { path: { address: vars.address } },
          body: { decision: vars.decision },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["capture-senders"] });
      toast.show(t("settings.saved"));
    },
  });
}

function useWithdrawDecision() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    mutationFn: async (address: string) => {
      unwrap(
        await api.DELETE("/capture/senders/{address}/decision", {
          params: { path: { address } },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["capture-senders"] });
      toast.show(t("settings.saved"));
    },
  });
}

export function CaptureSendersCard() {
  const t = useT();
  const query = useSenders();
  const setDecision = useSetDecision();
  const withdraw = useWithdrawDecision();
  // Keeping a sender out destroys the mail they already brought in, so it asks
  // first. Readmitting one does not, because nothing is lost by it.
  const [keepingOut, setKeepingOut] = useState<string | null>(null);

  return (
    <Panel title={t("senders.title")}>
      <PanelBody>
        <PanelIntro>{t("senders.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("senders.title")}>
        {(list) =>
          list.data.length === 0 ? (
            <PanelBody>
              <EmptyState>{t("senders.emptyBody")}</EmptyState>
            </PanelBody>
          ) : (
            <SenderTable
              rows={list.data}
              verbs={{ setDecision, withdraw, keepOut: setKeepingOut }}
            />
          )
        }
      </QueryGate>
      {(setDecision.isError || withdraw.isError) && (
        <PanelBody>
          <ErrorLine error={setDecision.error ?? withdraw.error} />
        </PanelBody>
      )}
      <ConfirmModal
        open={keepingOut !== null}
        onClose={() => setKeepingOut(null)}
        title={t("senders.keepOutTitle")}
        confirmLabel={t("senders.keepOutConfirm")}
        confirmVariant="danger"
        pending={setDecision.isPending}
        onConfirm={() => {
          if (keepingOut) {
            setDecision.mutate(
              { address: keepingOut, decision: "keep_out" },
              { onSuccess: () => setKeepingOut(null) },
            );
          }
        }}
      >
        {t("senders.keepOutBody")}
      </ConfirmModal>
      <AwaitingDecision />
    </Panel>
  );
}

type SenderVerbs = Readonly<{
  setDecision: ReturnType<typeof useSetDecision>;
  withdraw: ReturnType<typeof useWithdrawDecision>;
  keepOut: (address: string) => void;
}>;

function SenderTable({
  rows,
  verbs,
}: Readonly<{ rows: SenderDecision[]; verbs: SenderVerbs }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = viewerZone();
  const columns: DataTableColumn<SenderDecision>[] = [
    {
      key: "address",
      header: t("senders.colSender"),
      grow: true,
      render: (row) => (
        <span className="capture-senders-address">{row.address}</span>
      ),
    },
    {
      key: "decision",
      header: t("senders.colDecision"),
      render: (row) => <DecisionCell row={row} />,
    },
    {
      key: "record",
      header: t("senders.colRecord"),
      render: (row) =>
        row.contact_id ? (
          <EntityRef
            kind="contact"
            id={row.contact_id}
            name={row.contact_name}
          />
        ) : (
          "—"
        ),
      folded: (row) => (row.contact_id ? undefined : t("senders.recordNo")),
    },
    {
      key: "messages",
      header: t("senders.colMessages"),
      align: "end",
      render: (row) => formatNumber(row.message_count, locale),
      folded: (row) =>
        plural("senders.messages", row.message_count, {
          count: formatNumber(row.message_count, locale),
        }),
    },
    {
      key: "lastSeen",
      header: t("senders.colLastSeen"),
      align: "end",
      render: (row) =>
        row.last_seen_at ? formatDateTime(row.last_seen_at, locale, zone) : "—",
      folded: (row) =>
        row.last_seen_at
          ? t("senders.lastSeen", {
              date: formatDateTime(row.last_seen_at, locale, zone),
            })
          : null,
    },
    {
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (row) => <SenderMenu row={row} verbs={verbs} />,
    },
  ];
  return (
    <DataTable<SenderDecision>
      bleed
      fold
      label={t("senders.title")}
      rows={rows}
      rowKey={(row) => row.address}
      columns={columns}
    />
  );
}

// The two answers the contract takes, and taking yours back. A sender already
// admitted needs no readmitting, so only the other answer is offered.
function SenderMenu({
  row,
  verbs: { setDecision, withdraw, keepOut },
}: Readonly<{ row: SenderDecision; verbs: SenderVerbs }>) {
  const t = useT();
  const offersBusiness =
    !admitted.has(row.kind ?? "") && row.decision !== "business";
  const offersKeepOut = row.decision !== "keep_out";
  if (!offersBusiness && !offersKeepOut && !row.overruled) {
    return null;
  }
  return (
    <span className="cell-actions">
      <OverflowMenu label={t("table.rowActions", { name: row.address })}>
        {offersBusiness && (
          <Button
            disabled={setDecision.isPending}
            onClick={() =>
              setDecision.mutate({ address: row.address, decision: "business" })
            }
          >
            {t("senders.markBusiness")}
          </Button>
        )}
        {offersKeepOut && (
          <Button aria-haspopup="dialog" onClick={() => keepOut(row.address)}>
            {t("senders.keepOut")}
          </Button>
        )}
        {row.overruled && (
          <Button
            disabled={withdraw.isPending}
            onClick={() => withdraw.mutate(row.address)}
          >
            {t("senders.withdraw")}
          </Button>
        )}
      </OverflowMenu>
    </span>
  );
}

// Drawn only when something waits, or when the read failed: an empty group
// under the senders list would be a second empty state saying what the first
// already said, and one flashed while loading would say it and take it back.
function AwaitingDecision() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = viewerZone();
  const query = useAwaitingDecision();
  if (query.isPending || (query.isSuccess && query.data.data.length === 0)) {
    return null;
  }
  const name = (row: AwaitingContact) =>
    row.display_name ?? row.emails[0] ?? null;
  const open = (row: AwaitingContact) =>
    navigate({ screen: "contacts", id: row.contact_id });
  const openLabel = (row: AwaitingContact) => {
    const named = name(row);
    return named
      ? t("senders.waiting.openNamed", { name: named })
      : t("senders.waiting.openUnnamed");
  };
  return (
    <>
      <PanelGroupHead title={t("senders.waiting.title")} level="h3" />
      <PanelBody>
        <PanelIntro>{t("senders.waiting.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("senders.waiting.title")}>
        {(list) => (
          <DataTable<AwaitingContact>
            bleed
            fold
            label={t("senders.waiting.title")}
            rows={list.data}
            rowKey={(row) => row.contact_id}
            onRowClick={open}
            columns={[
              {
                key: "contact",
                header: t("senders.waiting.colContact"),
                render: (row) => (
                  <CellStack>
                    <EntityRef
                      kind="contact"
                      id={row.contact_id}
                      name={name(row)}
                      asText
                    />
                    {row.emails.length > 0 && (
                      <span className="t-caption capture-senders-address">
                        {row.emails[0]}
                        {row.emails.length > 1 &&
                          ` ${plural("senders.waiting.moreAddresses", row.emails.length - 1, { count: formatNumber(row.emails.length - 1, locale) })}`}
                      </span>
                    )}
                  </CellStack>
                ),
              },
              {
                key: "since",
                header: t("senders.waiting.colSince"),
                align: "end",
                render: (row) => formatDate(row.captured_at, locale, zone),
              },
              {
                key: "open",
                header: t("table.actions"),
                headerHidden: true,
                align: "end",
                fold: "end",
                render: (row) => (
                  <RowOpen label={openLabel(row)} onOpen={() => open(row)} />
                ),
              },
            ]}
          />
        )}
      </QueryGate>
    </>
  );
}

// What was decided, and by whom. A seat's own answer outranks the classifier's
// and says so, so the row reads as a correction.
function DecisionCell({ row }: Readonly<{ row: SenderDecision }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  if (row.overruled) {
    return (
      <CellStack>
        <Badge tone={row.decision === "business" ? "success" : "danger"}>
          {row.decision === "business"
            ? t("senders.kind.business")
            : t("senders.kind.keptOut")}
        </Badge>
        <span className="t-caption">{t("senders.byYou")}</span>
      </CellStack>
    );
  }
  const kind = row.kind ?? "";
  const known = kindLabel[kind];
  return (
    <CellStack>
      <Badge tone={admitted.has(kind) ? "success" : undefined}>
        {known
          ? t(known)
          : t(kind ? "senders.kind.other" : "senders.kind.undecided")}
      </Badge>
      {/* The deadline, not just the verdict: a personal verdict changes nothing
          on the page until the mail goes, so the date is what an owner acts on. */}
      {row.deletes_at && (
        <span className="t-caption">
          {t("senders.deletesOn", {
            date: formatDate(row.deletes_at, locale, zone),
          })}
        </span>
      )}
    </CellStack>
  );
}
