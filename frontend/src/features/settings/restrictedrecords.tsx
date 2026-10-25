import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  EmptyState,
  Field,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { CardBoundary } from "../../design-system/cardboundary";
import { CellStack } from "../../design-system/cellstack";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate, QueryStates } from "../../design-system/querystates";
import { formatDate, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan, useCanWrite } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { useMe } from "../../lib/session";
import { isUuid } from "../../lib/uuid";
import { humanizeToken } from "./audit";

// The Held lane reads through the retention ladder's authority. A role that may
// not see how long records are kept may not see which are kept either.

export type RestrictedRecord = components["schemas"]["RestrictedRecord"];

export const RESTRICTED_RECORDS_KEY = ["retention", "restrictions"] as const;

// The obligation's class is the first token of the server's reason
// ("commercial_correspondence · §257 HGB / §147 AO"); the statute after the
// separator is shown as written, because it is the citation and not a label.
function splitReason(reason: string): { cls: string; basis: string } {
  const [cls = "", ...rest] = reason.split(" · ");
  return { cls, basis: rest.join(" · ") };
}

// The classes the schema admits today, by name. A class this build has not
// heard of renders as its own token rather than a missing key — a newer
// server must not make the list unreadable.
const CLASS_LABEL: Readonly<Record<string, MessageKey>> = {
  commercial_correspondence: "restricted.class.commercialCorrespondence",
};

// The interaction kinds the timeline knows, by name; same fallback.
const KIND_LABEL: Readonly<Record<string, MessageKey>> = {
  email: "restricted.kind.email",
  call: "restricted.kind.call",
  meeting: "restricted.kind.meeting",
  message: "restricted.kind.message",
};

// The two decisions a controller can make about a held record: RELEASE ends
// the obligation by erasing the record — it does not return it to ordinary
// use, because the erasure request it suspended is still outstanding — and
// PIN places a record under the floor the derivation missed. Both are the
// same shape on screen (a typed reason, then an irreversible act), so they
// are one component with the words swapped rather than two.
type Override = "release" | "pin";

// A release acts on a row of the list. A pin names a record the derivation
// missed, so no row holds it: the controller types its id from the audit trail.
type OverrideTarget = Readonly<{ activityId: string }>;

type OverrideDecision = Readonly<{
  activityId: string;
  kind: Override;
  reason: string;
}>;

function OverrideModal({
  open,
  target,
  kind,
  onClose,
}: Readonly<{
  open: boolean;
  // The row a release acts on; a pin asks for its record in the dialog.
  target: OverrideTarget | null;
  kind: Override;
  onClose: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [reason, setReason] = useState("");
  const [typedId, setTypedId] = useState("");
  const activityId = kind === "pin" ? typedId.trim() : target?.activityId;
  // A malformed id keeps the irreversible confirm shut. Shape only: whether
  // the record exists is the server's answer.
  const idIsWellFormed = isUuid(typedId.trim());
  const idIsMalformed =
    kind === "pin" && typedId.trim() !== "" && !idIsWellFormed;

  const decide = useMutation({
    mutationFn: async (decision: OverrideDecision) => {
      const path =
        decision.kind === "release"
          ? ("/retention/restrictions/{activityId}/release" as const)
          : ("/retention/restrictions/{activityId}/pin" as const);
      unwrap(
        await api.POST(path, {
          params: { path: { activityId: decision.activityId } },
          body: { reason: decision.reason },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: RESTRICTED_RECORDS_KEY });
      setReason("");
      setTypedId("");
      onClose();
    },
  });

  // The reason is what makes this a decision rather than a toggle, so the
  // confirm waits for one: the server refuses a blank one.
  return (
    <ConfirmModal
      open={open}
      onClose={() => {
        setReason("");
        setTypedId("");
        decide.reset();
        onClose();
      }}
      title={t(`restricted.${kind}.title`)}
      confirmLabel={t(`restricted.${kind}.confirm`)}
      confirmVariant="danger"
      confirmDisabled={
        reason.trim() === "" || (kind === "pin" && !idIsWellFormed)
      }
      onConfirm={() =>
        activityId && decide.mutate({ activityId, kind, reason })
      }
      pending={decide.isPending}
      error={decide.error ? problemMessageOf(decide.error, t) : null}
    >
      <p>{t(`restricted.${kind}.body`)}</p>
      {kind === "pin" && (
        <Field
          label={t("restricted.pin.idLabel")}
          hint={t("restricted.pin.idHint")}
          error={idIsMalformed ? t("restricted.pin.idMalformed") : undefined}
          required
        >
          {(control) => (
            <TextInput
              {...control}
              value={typedId}
              onChange={(event) => setTypedId(event.target.value)}
            />
          )}
        </Field>
      )}
      <Field
        label={t("restricted.reasonLabel")}
        hint={t("restricted.reasonHint")}
        required
      >
        {(control) => (
          <Textarea
            {...control}
            rows={3}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}

// Shared by the lane and the Requests panel's count, so both read one cache entry.
export function useRestrictedRecords(enabled: boolean) {
  return useQuery({
    queryKey: RESTRICTED_RECORDS_KEY,
    enabled,
    queryFn: async () => {
      return unwrap(await api.GET("/retention/restrictions"));
    },
  });
}

// The pin verb for the Requests panel's head, shown to the authority that can carry it out.
export function PinRecordAction() {
  const t = useT();
  const canRead = useCan("retention_policy", "read");
  const canDecide = useCanWrite("retention_policy", "update");
  const [pinning, setPinning] = useState(false);
  if (!canRead || !canDecide) {
    return null;
  }
  return (
    <>
      <Button aria-haspopup="dialog" onClick={() => setPinning(true)}>
        {t("restricted.pin.action")}
      </Button>
      <OverrideModal
        open={pinning}
        target={null}
        kind="pin"
        onClose={() => setPinning(false)}
      />
    </>
  );
}

// The restricted-records lane of the Requests panel: its bands, not a panel.
export function RestrictedRecordsCard() {
  const t = useT();
  const plural = usePlural();
  const me = useMe();
  const { locale } = useLocale();
  const tz = viewerZone();
  // The held activity's own date is the record's, and a `day` one has no
  // instant to move into the reader's zone.
  const recordZone = useRecordZone();
  const canRead = useCan("retention_policy", "read");
  // Reading what is held and DECIDING about it are separate grants, so the
  // row action appears only for the authority that can carry it out.
  const canDecide = useCanWrite("retention_policy", "update");
  const [releasing, setReleasing] = useState<OverrideTarget | null>(null);
  const records = useRestrictedRecords(canRead);

  if (!canRead) {
    return (
      <PanelBody>
        <PanelIntro>{t("restricted.sub")}</PanelIntro>
        <QueryGate query={me} pendingLabel={t("restricted.title")}>
          {() => <EmptyState>{t("restricted.withheld")}</EmptyState>}
        </QueryGate>
      </PanelBody>
    );
  }

  const kindOf = (row: RestrictedRecord) => {
    const label = KIND_LABEL[row.kind];
    return label ? t(label) : humanizeToken(row.kind);
  };
  const columns: DataTableColumn<RestrictedRecord>[] = [
    {
      key: "record",
      header: t("restricted.kind"),
      render: (row) => {
        const removed = (row.redacted_fields ?? []).length;
        return (
          <CellStack>
            <span>{kindOf(row)}</span>
            <span className="t-caption">
              {formatDate(row.occurred_at, locale, recordZone)}
            </span>
            <span className="t-caption">
              {removed === 0
                ? t("restricted.nothingRedacted")
                : plural("restricted.redactedCount", removed, {
                    count: formatNumber(removed, locale),
                  })}
            </span>
          </CellStack>
        );
      },
    },
    {
      key: "deals",
      header: t("restricted.deals"),
      grow: true,
      // A project qualifies its correspondence on its own, so a row held by a
      // project alone has an empty `deals`; both name the qualifying transaction.
      render: (row) => {
        const qualifying = [
          ...row.deals.map((deal) => deal.name),
          ...(row.projects ?? []).map((project) => project.name),
        ];
        return qualifying.length === 0
          ? t("restricted.noDeal")
          : qualifying.join(", ");
      },
    },
    {
      key: "reason",
      header: t("restricted.reason"),
      render: (row) => {
        const { cls, basis } = splitReason(row.reason);
        return (
          <CellStack>
            <span>
              {CLASS_LABEL[cls] ? t(CLASS_LABEL[cls]) : humanizeToken(cls)}
            </span>
            <span className="t-caption">{basis}</span>
          </CellStack>
        );
      },
    },
    {
      key: "until",
      header: t("restricted.until"),
      render: (row) => (
        <CellStack>
          <span>{formatDate(row.restricted_until, locale, tz)}</span>
          <span className="t-caption">
            {t("restricted.since", {
              date: formatDate(row.restricted_at, locale, tz),
            })}
          </span>
        </CellStack>
      ),
    },
  ];
  if (canDecide) {
    columns.push({
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      fold: "end",
      align: "end",
      // Not the danger tone: the irreversible act is the dialog's confirm, and
      // a red verb on every row out-shouts the table it sits in.
      render: (row) => (
        <span className="cell-actions">
          <Button
            aria-label={t("restricted.release.actionNamed", {
              kind: kindOf(row),
              date: formatDate(row.occurred_at, locale, recordZone),
            })}
            aria-haspopup="dialog"
            onClick={() => setReleasing({ activityId: row.activity_id })}
          >
            {t("restricted.release.action")}
          </Button>
        </span>
      ),
    });
  }

  const held = records.data?.data ?? [];
  const shown = records.isSuccess && held.length > 0;
  return (
    <>
      <PanelBody>
        <PanelIntro>{t("restricted.sub")}</PanelIntro>
        {!shown && (
          <QueryStates query={records} pendingLabel={t("restricted.title")}>
            <EmptyState>{t("restricted.empty")}</EmptyState>
          </QueryStates>
        )}
      </PanelBody>
      <CardBoundary>
        {shown && (
          <DataTable
            label={t("restricted.heldLabel")}
            bleed
            fold
            columns={columns}
            rows={held}
            rowKey={(row) => row.activity_id}
          />
        )}
        <OverrideModal
          open={releasing !== null}
          target={releasing}
          kind="release"
          onClose={() => setReleasing(null)}
        />
      </CardBoundary>
    </>
  );
}
