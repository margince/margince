import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import {
  Button,
  EmptyState,
  Field,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { CardBoundary } from "../design-system/cardboundary";
import { CellStack } from "../design-system/cellstack";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatDate, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { humanizeToken } from "./audit";
import {
  problemMessageOf,
  QueryGate,
  QueryStates,
  unwrap,
  useMe,
} from "./common";
import "./retention.css";

// Settings → Privacy → Restricted records (A165/ADR-0114 §4): what a
// statutory obligation is holding after an erasure — which record, why, and
// until when — stated without the correspondence itself. The audit log proves
// what happened; this answers what is being held right now, which is the
// question a supervisory authority asks the controller.
//
// It reads through the same authority as the retention ladder above it, so a
// role that may not see how long records are kept may not see which are being
// kept either.

export type RestrictedRecord = components["schemas"]["RestrictedRecord"];

export const RESTRICTED_RECORDS_KEY = ["retention", "restrictions"] as const;

// A pin names its record by id, and the id has to be well-formed BEFORE the
// confirm opens: the dialog behind it warns about an irreversible act on a
// record the controller cannot otherwise see, so letting a typo through means
// reading that warning, typing a reason, and only then learning they named
// nothing. Shape only — whether the record exists is the server's answer.
const RECORD_ID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// The obligation's class is the first token of the server's reason
// ("commercial_correspondence · §257 HGB / §147 AO"); the statute after the
// separator is shown as written, because it is the citation and not a label.
function splitReason(reason: string): { cls: string; basis: string } {
  const [cls, ...rest] = reason.split(" · ");
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

// What an override acts on: a row from the list for a release, a record id
// typed in for a pin. A pin is BY DEFINITION about a record this list does not
// hold — the derivation missed it — so there is no row to hang the action off,
// and the id is how a controller names it. They reach it from the audit trail
// or the record page, where the id is on screen.
type OverrideTarget = Readonly<{ activityId: string }>;

type OverrideDecision = Readonly<{
  target: OverrideTarget | null;
  kind: Override;
  reason: string;
}>;

function OverrideModal({
  target,
  kind,
  onClose,
}: Readonly<{
  target: OverrideTarget | null;
  kind: Override;
  onClose: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [reason, setReason] = useState("");

  const decide = useMutation({
    mutationFn: async (decision: OverrideDecision) => {
      // A decision about nothing is a failure, never a success that closes the dialog.
      if (!decision.target) {
        throw new Error("no record chosen for this decision");
      }
      const path =
        decision.kind === "release"
          ? ("/retention/restrictions/{activityId}/release" as const)
          : ("/retention/restrictions/{activityId}/pin" as const);
      unwrap(
        await api.POST(path, {
          params: { path: { activityId: decision.target.activityId } },
          body: { reason: decision.reason },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: RESTRICTED_RECORDS_KEY });
      setReason("");
      onClose();
    },
  });

  // The reason is what makes this a decision rather than a toggle, so the
  // confirm stays disabled until one is actually typed — the server refuses a
  // blank one, and a button that fires into that refusal teaches nothing.
  return (
    <ConfirmModal
      open={target !== null}
      onClose={() => {
        setReason("");
        onClose();
      }}
      title={t(`restricted.${kind}.title`)}
      confirmLabel={t(`restricted.${kind}.confirm`)}
      confirmVariant="danger"
      confirmDisabled={reason.trim() === ""}
      onConfirm={() => decide.mutate({ target, kind, reason })}
      pending={decide.isPending}
      error={decide.error ? problemMessageOf(decide.error, t) : null}
    >
      <p>{t(`restricted.${kind}.body`)}</p>
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

export function RestrictedRecordsCard() {
  const t = useT();
  const plural = usePlural();
  const me = useMe();
  const { locale } = useLocale();
  const tz = viewerZone();
  const canRead = useCan("retention_policy", "read");
  // Reading what is held and DECIDING about it are separate grants, so the
  // row action appears only for the authority that can carry it out.
  const canDecide = useCanWrite("retention_policy", "update");
  const [releasing, setReleasing] = useState<OverrideTarget | null>(null);
  const [pinning, setPinning] = useState<OverrideTarget | null>(null);
  const [pinId, setPinId] = useState("");
  const pinErrorId = useId();
  const pinIdIsWellFormed = RECORD_ID_RE.test(pinId.trim());
  const pinIdIsMalformed = pinId.trim() !== "" && !pinIdIsWellFormed;

  const records = useQuery({
    queryKey: RESTRICTED_RECORDS_KEY,
    enabled: canRead,
    queryFn: async () => {
      return unwrap(await api.GET("/retention/restrictions"));
    },
  });

  if (!canRead) {
    return (
      <Panel title={t("restricted.title")}>
        <PanelBody>
          <PanelIntro>{t("restricted.sub")}</PanelIntro>
          <QueryGate query={me} pendingLabel={t("restricted.title")}>
            {() => <EmptyState>{t("restricted.withheld")}</EmptyState>}
          </QueryGate>
        </PanelBody>
      </Panel>
    );
  }

  const kindOf = (row: RestrictedRecord) =>
    KIND_LABEL[row.kind] ? t(KIND_LABEL[row.kind]) : humanizeToken(row.kind);
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
              {formatDate(row.occurred_at, locale, tz)}
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
              date: formatDate(row.occurred_at, locale, tz),
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
  return (
    <Panel title={t("restricted.title")}>
      <PanelBody>
        <PanelIntro>{t("restricted.sub")}</PanelIntro>
      </PanelBody>
      <CardBoundary>
        {records.isSuccess && held.length > 0 ? (
          <DataTable
            label={t("restricted.heldLabel")}
            bleed
            fold
            columns={columns}
            rows={held}
            rowKey={(row) => row.activity_id}
          />
        ) : (
          <PanelBody>
            <QueryStates query={records} pendingLabel={t("restricted.title")}>
              <EmptyState>{t("restricted.empty")}</EmptyState>
            </QueryStates>
          </PanelBody>
        )}
        {/* One input and its verb, so a row; the reason and its warning are the
            confirm dialog behind it. In a body, whose seam parts it from the table. */}
        {canDecide && (
          <PanelBody>
            <SettingList>
              <SettingRow
                label={t("restricted.pin.action")}
                description={t("restricted.pin.idHint")}
                control={(control) => (
                  <form
                    className="restricted-pin"
                    onSubmit={(event) => {
                      event.preventDefault();
                      // The disabled button is a hint; the submit itself refuses a malformed id.
                      if (pinIdIsWellFormed) {
                        setPinning({ activityId: pinId.trim() });
                      }
                    }}
                  >
                    <TextInput
                      {...control}
                      // A malformed id adds the refusal to the row's description,
                      // so a reader hears the rule and how they broke it.
                      aria-describedby={
                        [
                          control["aria-describedby"],
                          pinIdIsMalformed ? pinErrorId : null,
                        ]
                          .filter(Boolean)
                          .join(" ") || undefined
                      }
                      aria-invalid={pinIdIsMalformed || undefined}
                      value={pinId}
                      onChange={(event) => setPinId(event.target.value)}
                      placeholder={t("restricted.pin.idPlaceholder")}
                    />
                    <Button type="submit" disabled={!pinIdIsWellFormed}>
                      {t("restricted.pin.submit")}
                    </Button>
                    {/* Re-announced on each keystroke, it would drown the field being typed. */}
                    {pinIdIsMalformed && (
                      <ErrorLine id={pinErrorId} standing>
                        {t("restricted.pin.idMalformed")}
                      </ErrorLine>
                    )}
                  </form>
                )}
              />
            </SettingList>
          </PanelBody>
        )}
        <OverrideModal
          target={releasing}
          kind="release"
          onClose={() => setReleasing(null)}
        />
        <OverrideModal
          target={pinning}
          kind="pin"
          onClose={() => {
            setPinning(null);
            setPinId("");
          }}
        />
      </CardBoundary>
    </Panel>
  );
}
