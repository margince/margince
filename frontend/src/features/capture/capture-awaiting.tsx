import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { EntityRef } from "../../components/entityref";
import { Button, OverflowMenu } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable } from "../../design-system/datatable";
import { RowOpen } from "../../design-system/rowopen";
import { useToast } from "../../design-system/toast";
import { formatDate, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemMessageOf, throwProblem, unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { useViewerZone } from "../../lib/viewerzone";
import {
  AWAITING_KEY,
  invalidateSenderLists,
  putSenderDecision,
} from "./capture-senderquery";

// The contacts this seat's mail created that only they can see until their
// sender is settled. Each row offers the answers that settle it in place. The admin capture-health card counts the same set for this mailbox.

type AwaitingContact = components["schemas"]["CaptureAwaitingContact"];
type AwaitingState = components["schemas"]["CaptureAwaitingAddress"]["state"];

export function useAwaitingDecision() {
  return useQuery({
    queryKey: AWAITING_KEY,
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

// Why one address waits, in the reader's words. A state this screen does not
// know reads as the owner's to settle, since nothing here can process it.
const stateWhy: Record<string, MessageKey> = {
  decided: "senders.waiting.why.decided",
  classifying: "senders.waiting.why.classifying",
  unsure: "senders.waiting.why.unsure",
  queued: "senders.waiting.why.queued",
  unjudgeable: "senders.waiting.why.unjudgeable",
};
const processing = new Set<AwaitingState>(["decided", "classifying", "queued"]);

// What a press asks for, held until the dialog that names its effects is
// confirmed. Each names one address, so a contact with several cannot have an
// answer land on one the reader did not pick.
type Resolution =
  | { kind: "business"; contact: AwaitingContact; address: string }
  | { kind: "keep_out"; contact: AwaitingContact; address: string }
  | { kind: "remove"; contact: AwaitingContact };

function useResolve() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    mutationFn: async (r: Resolution) => {
      if (r.kind === "remove") {
        await archiveContact(r.contact.contact_id);
        return;
      }
      await putSenderDecision(r.address, r.kind);
      // Business settles the sender for future mail; the owner's own publish
      // is what moves this contact now, rather than at the next verdict pass.
      if (r.kind === "business") {
        unwrap(
          await api.POST("/contacts/{id}/publish", {
            params: { path: { id: r.contact.contact_id } },
          }),
        );
      }
    },
    onSuccess: () => {
      invalidateSenderLists(queryClient);
      toast.show(t("settings.saved"));
    },
  });
}

// Archiving guards on the version the reader saw; the list carries none, so it
// is read here, right before the write that depends on it.
async function archiveContact(id: string) {
  const contact = unwrap(
    await api.GET("/contacts/{id}", { params: { path: { id } } }),
  );
  unwrap(
    await api.DELETE("/contacts/{id}", {
      params: { path: { id }, ...ifMatch(requireVersion(contact.version)) },
    }),
  );
}

/** Contacts only the reader can see until their sender is decided. */
export function AwaitingContacts({
  rows,
}: Readonly<{ rows: AwaitingContact[] }>) {
  const t = useT();
  const resolve = useResolve();
  const [asked, setAsked] = useState<Resolution | null>(null);
  const close = () => {
    setAsked(null);
    resolve.reset();
  };
  return (
    <>
      <AwaitingTable rows={rows} onAsk={setAsked} />
      <ConfirmModal
        open={asked !== null}
        onClose={close}
        title={asked ? t(confirmTitle[asked.kind]) : ""}
        confirmLabel={asked ? t(confirmLabel[asked.kind]) : ""}
        confirmVariant={asked?.kind === "keep_out" ? "danger" : "primary"}
        pending={resolve.isPending}
        error={resolve.isError ? problemMessageOf(resolve.error, t) : null}
        onConfirm={() => {
          if (asked) {
            resolve.mutate(asked, { onSuccess: () => setAsked(null) });
          }
        }}
      >
        {asked && <ResolutionEffects asked={asked} />}
      </ConfirmModal>
    </>
  );
}

const confirmTitle: Record<Resolution["kind"], MessageKey> = {
  business: "senders.waiting.businessTitle",
  keep_out: "senders.keepOutTitle",
  remove: "senders.waiting.removeTitle",
};
const confirmLabel: Record<Resolution["kind"], MessageKey> = {
  business: "senders.markBusiness",
  keep_out: "senders.keepOutConfirm",
  remove: "senders.waiting.removeConfirm",
};

// The effects are said before the write, and each names what it leaves alone.
// So a dismissal never reads as destroying mail or deciding the sender.
function ResolutionEffects({ asked }: Readonly<{ asked: Resolution }>) {
  const t = useT();
  if (asked.kind === "remove") {
    return <p>{t("senders.waiting.removeBody")}</p>;
  }
  return (
    <>
      <p className="t-caption">{asked.address}</p>
      <p>
        {asked.kind === "business"
          ? t("senders.waiting.businessBody")
          : t("senders.keepOutBody")}
      </p>
    </>
  );
}

function AwaitingTable({
  rows,
  onAsk,
}: Readonly<{
  rows: AwaitingContact[];
  onAsk: (r: Resolution) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  return (
    <DataTable<AwaitingContact>
      fold
      label={t("senders.waiting.title")}
      rows={rows}
      rowKey={(row) => row.contact_id}
      columns={[
        {
          key: "contact",
          header: t("senders.waiting.colContact"),
          grow: true,
          render: (row) => <WaitingContact row={row} />,
        },
        {
          key: "since",
          header: t("senders.waiting.colSince"),
          align: "end",
          render: (row) => formatDate(row.captured_at, locale, zone),
        },
        {
          key: "verbs",
          header: t("table.actions"),
          headerHidden: true,
          align: "end",
          fold: "end",
          render: (row) => <WaitingVerbs row={row} onAsk={onAsk} />,
        },
      ]}
    />
  );
}

function contactName(row: AwaitingContact) {
  return row.display_name ?? row.emails[0] ?? null;
}

// The contact, its first address, and why it waits. The reason says whose move
// it is: Margince's when a pass will settle it, the reader's otherwise.
function WaitingContact({ row }: Readonly<{ row: AwaitingContact }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const states = row.addresses.map((a) => a.state);
  const ours = states.some((s) => processing.has(s));
  const reason =
    states.find((s) => processing.has(s) === ours) ?? "unjudgeable";
  const more = row.emails.length - 1;
  return (
    <CellStack>
      <EntityRef
        kind="contact"
        id={row.contact_id}
        name={contactName(row)}
        asText
      />
      {row.emails.length > 0 && (
        <span className="t-caption capture-senders-address">
          {row.emails[0]}
          {more > 0 &&
            ` ${plural("senders.waiting.moreAddresses", more, { count: formatNumber(more, locale) })}`}
        </span>
      )}
      <span className="t-caption">
        {t(ours ? "senders.waiting.onMargince" : "senders.waiting.onYou")}{" "}
        {t(stateWhy[reason] ?? "senders.waiting.why.unjudgeable")}
      </span>
    </CellStack>
  );
}

// One answer per address still open, so the reader picks which address an
// answer is about. An address already answered offers nothing to answer again.
function WaitingVerbs({
  row,
  onAsk,
}: Readonly<{ row: AwaitingContact; onAsk: (r: Resolution) => void }>) {
  const t = useT();
  const name = contactName(row);
  const several = row.addresses.length > 1;
  const open = () => navigate({ screen: "contacts", id: row.contact_id });
  return (
    <span className="cell-actions">
      <OverflowMenu
        label={t("table.rowActions", {
          name: name ?? t("senders.waiting.openUnnamed"),
        })}
      >
        {row.addresses
          .filter((a) => a.state !== "decided")
          .flatMap((a) => [
            <Button
              key={`business-${a.email}`}
              aria-haspopup="dialog"
              onClick={() =>
                onAsk({ kind: "business", contact: row, address: a.email })
              }
            >
              {several
                ? t("senders.waiting.businessFor", { address: a.email })
                : t("senders.markBusiness")}
            </Button>,
            <Button
              key={`keep-${a.email}`}
              aria-haspopup="dialog"
              onClick={() =>
                onAsk({ kind: "keep_out", contact: row, address: a.email })
              }
            >
              {several
                ? t("senders.waiting.keepOutFor", { address: a.email })
                : t("senders.keepOut")}
            </Button>,
          ])}
        <Button
          aria-haspopup="dialog"
          onClick={() => onAsk({ kind: "remove", contact: row })}
        >
          {t("senders.waiting.remove")}
        </Button>
      </OverflowMenu>
      <RowOpen
        label={
          name
            ? t("senders.waiting.openNamed", { name })
            : t("senders.waiting.openUnnamed")
        }
        onOpen={open}
      />
    </span>
  );
}
