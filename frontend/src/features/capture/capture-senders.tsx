import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Badge, Button, OverflowMenu } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { useToast } from "../../design-system/toast";
import { useTruncationTooltip } from "../../design-system/tooltip";
import { formatActivityWhen } from "../../format/activitywhen";
import { formatDate, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { throwProblem, unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import {
  invalidateSenderLists,
  kindLabel,
  putSenderDecision,
  SENDERS_KEY,
  type SenderAnswer,
} from "./capture-senderquery";
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

// The classifier's vocabulary, in the reader's words. A kind absent from the
// map is one the server learned to say and this screen has not.
// Which kinds mean "this sender's mail is in the CRM as a contact". The tone
// carries it at a glance down the column; the words still say it, because
// colour is never the only signal.
const admitted = new Set(["contact", "role_mailbox", "company_sender"]);

export function useSenders() {
  return useQuery({
    queryKey: SENDERS_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/capture/senders", {
        params: { query: { limit: 1 } },
      });
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
    mutationFn: (vars: { address: string; decision: SenderAnswer }) =>
      putSenderDecision(vars.address, vars.decision),
    onSuccess: () => {
      invalidateSenderLists(queryClient);
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
      invalidateSenderLists(queryClient);
      toast.show(t("settings.saved"));
    },
  });
}

export function SenderDecisions({
  rows,
}: Readonly<{ rows: SenderDecision[] }>) {
  const t = useT();
  const setDecision = useSetDecision();
  const withdraw = useWithdrawDecision();
  // Keeping a sender out destroys the mail they already brought in, so it asks
  // first. Readmitting one does not, because nothing is lost by it.
  const [keepingOut, setKeepingOut] = useState<string | null>(null);

  return (
    <>
      <SenderTable
        rows={rows}
        verbs={{ setDecision, withdraw, keepOut: setKeepingOut }}
      />
      <ErrorLine
        error={
          setDecision.isError || withdraw.isError
            ? (setDecision.error ?? withdraw.error)
            : undefined
        }
      />
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
    </>
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
  const zone = useViewerZone();
  const columns: DataTableColumn<SenderDecision>[] = [
    {
      key: "address",
      header: t("senders.colSender"),
      grow: true,
      render: (row) => <SenderAddress address={row.address} />,
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
        row.last_seen_at
          ? formatActivityWhen(
              row.last_seen_at,
              row.last_seen_precision ?? "instant",
              locale,
              zone,
            )
          : "—",
      folded: (row) =>
        row.last_seen_at
          ? t("senders.lastSeen", {
              date: formatActivityWhen(
                row.last_seen_at,
                row.last_seen_precision ?? "instant",
                locale,
                zone,
              ),
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
      fold
      label={t("senders.title")}
      rows={rows}
      rowKey={(row) => row.address}
      columns={columns}
    />
  );
}

// One line per sender, cut short with the whole address on hover and focus.
// Broken at any character instead, a generated bounce address stood as a
// column of fragments that read as several senders.
function SenderAddress({ address }: Readonly<{ address: string }>) {
  const { ref, trigger, tip } = useTruncationTooltip<HTMLSpanElement>(address);
  return (
    <span className="capture-senders-sender" ref={ref} {...trigger}>
      {address}
      {tip}
    </span>
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

// What was decided, and by whom. A seat's own answer outranks the classifier's
// and says so, so the row reads as a correction.
function DecisionCell({ row }: Readonly<{ row: SenderDecision }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
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
