// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Why one record is on a list, in a drawer beside the members.
//
// A Live List answers with its filter, clause by clause, each judged for this
// record by the same SQL that decides who is a member, with the record's own
// value beside it — or "hidden" where the reader may not see that value. A
// Shortlist answers with who chose the record, when, and the note they left.

import { Check, CircleHelp, X } from "lucide-react";
import { useId, useState } from "react";
import { Button, Field, Textarea } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime, formatMoney, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import { problemMessageOf } from "./common";
import { operatorKey } from "./filterbuilder";
import { fieldLabel, useFilterVocabulary } from "./filterdata";
import {
  type List,
  type ListClauseVerdict,
  type ListExplanation,
  useChangeMember,
  useExplanation,
} from "./lists.queries";
import type { FilterOp } from "./segmentpredicate";

export function ListWhy({
  list,
  record,
  onClose,
}: Readonly<{
  list: List;
  record: Readonly<{ id: string; name: string }> | null;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const why = useExplanation(list.id, record?.id ?? null);
  return (
    <Modal
      open={record !== null}
      onClose={onClose}
      labelledBy={titleId}
      placement="right"
    >
      <Heading size="large" id={titleId} className="modal-title">
        {t("lists.why.title", { name: record?.name ?? "" })}
      </Heading>
      <SurfaceState
        state={
          why.isPending ? "loading" : why.isError ? "unavailable" : "ready"
        }
        emptyLabel={t("lists.why.loading")}
        loadingLabel={t("lists.why.loading")}
        loadingLines={4}
      >
        {why.data && list.list_type === "dynamic" ? (
          <LiveWhy list={list} why={why.data} />
        ) : why.data ? (
          <ChosenWhy list={list} why={why.data} onRemoved={onClose} />
        ) : null}
      </SurfaceState>
    </Modal>
  );
}

function LiveWhy({
  list,
  why,
}: Readonly<{ list: List; why: ListExplanation }>) {
  const t = useT();
  return (
    <div className="lists-why">
      <p className="lists-note">
        {why.member
          ? t("lists.why.liveMember")
          : why.eligible === false
            ? t("lists.why.notEligible")
            : t("lists.why.liveNotMember")}
      </p>
      {why.clauses && <Verdict node={why.clauses} list={list} />}
    </div>
  );
}

function VerdictMark({ result }: Readonly<{ result: boolean | null }>) {
  const t = useT();
  if (result === true) {
    return <Check aria-label={t("lists.why.met")} className="lists-why-met" />;
  }
  if (result === false) {
    // ds:ignore a verdict glyph drawn in the ink, not a failure line
    return <X aria-label={t("lists.why.unmet")} className="lists-why-unmet" />;
  }
  return (
    <CircleHelp
      aria-label={t("lists.why.unknown")}
      className="lists-why-unknown"
    />
  );
}

function Verdict({
  node,
  list,
}: Readonly<{ node: ListClauseVerdict; list: List }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const vocabulary = useFilterVocabulary(list.entity_type);
  if (node.join) {
    return (
      <div className="lists-why-group">
        <p className="t-caption">
          <VerdictMark result={node.result} />
          {node.join === "and" ? t("lists.why.all") : t("lists.why.any")}
        </p>
        <ul className="lists-why-clauses">
          {(node.children ?? []).map((child, index) => (
            // Clauses have no identity of their own; their place is it.
            // biome-ignore lint/suspicious/noArrayIndexKey: a clause is its position in the tree
            <li key={index}>
              <Verdict node={child} list={list} />
            </li>
          ))}
        </ul>
      </div>
    );
  }
  const field = vocabulary.data?.fields.find((f) => f.name === node.field);
  const label = field ? fieldLabel(field, t) : (node.field ?? "");
  const op = t(
    operatorKey((node.op ?? "eq") as FilterOp, field?.type ?? "text"),
  );
  const money =
    field?.type === "currency" && field.currency ? field.currency : undefined;
  const shown = (raw: unknown) =>
    money ? moneyText(raw, money, locale) : undefined;
  return (
    <p className="lists-why-clause">
      <VerdictMark result={node.result} />
      <span>
        {label} {op}{" "}
        <strong>
          {shown(node.operand) ?? operandText(node.operand, t, plural, locale)}
        </strong>
      </span>
      <span className="t-caption">
        {node.hidden
          ? t("lists.why.hidden")
          : t("lists.why.value", {
              value:
                node.value == null
                  ? t("lists.why.empty")
                  : (shown(node.value) ?? node.value),
            })}
      </span>
    </p>
  );
}

/**
 * A money operand or value — minor units of the field's currency — as the
 * reader reads money, or undefined for anything that is not an amount.
 */
export function moneyText(
  raw: unknown,
  currency: string,
  locale: Locale,
): string | undefined {
  const one = (v: unknown) =>
    typeof v === "number" || (typeof v === "string" && /^-?\d+$/.test(v))
      ? formatMoney(Number(v), currency, locale)
      : undefined;
  if (Array.isArray(raw)) {
    const each = raw.map(one);
    return each.every((v) => v !== undefined) ? each.join(", ") : undefined;
  }
  return one(raw);
}

/** A clause's operand as the filter states it. */
function operandText(
  operand: unknown,
  t: ReturnType<typeof useT>,
  plural: ReturnType<typeof usePlural>,
  locale: Locale,
): string {
  if (operand === true) {
    return t("filters.yes");
  }
  if (operand === false) {
    return t("filters.no");
  }
  if (Array.isArray(operand)) {
    return operand.join(", ");
  }
  if (
    operand !== null &&
    typeof operand === "object" &&
    "days_ago" in operand
  ) {
    const days = Number((operand as { days_ago: unknown }).days_ago);
    return plural("lists.why.daysAgo", days, {
      count: formatNumber(days, locale),
    });
  }
  return String(operand ?? "");
}

function ChosenWhy({
  list,
  why,
  onRemoved,
}: Readonly<{ list: List; why: ListExplanation; onRemoved: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  if (!why.member) {
    return <p className="lists-note">{t("lists.why.notChosen")}</p>;
  }
  return (
    <div className="lists-why">
      <p className="lists-note">
        {t("lists.why.chosen", {
          who: why.added_by_name ?? t("lists.why.someone"),
          when: why.added_at
            ? formatDateTime(why.added_at, locale, viewerZone())
            : "—",
        })}
      </p>
      {why.note && (
        <blockquote className="lists-why-note">{why.note}</blockquote>
      )}
      {list.can_edit && !list.archived_at && (
        <RemoveMemberAction
          list={list}
          recordId={why.entity_id}
          onRemoved={onRemoved}
        />
      )}
    </div>
  );
}

/** Taking a record off a Shortlist, with an optional note on why. */
function RemoveMemberAction({
  list,
  recordId,
  onRemoved,
}: Readonly<{ list: List; recordId: string; onRemoved: () => void }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [note, setNote] = useState("");
  const change = useChangeMember();
  return (
    <>
      <Button variant="danger" onClick={() => setOpen(true)}>
        {t("lists.remove")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("lists.removeTitle")}
        confirmLabel={t("lists.remove")}
        confirmVariant="danger"
        pending={change.isPending}
        error={change.isError ? problemMessageOf(change.error, t) : null}
        onConfirm={() =>
          change.mutate(
            {
              listId: list.id,
              entityType: list.entity_type,
              entityId: recordId,
              note,
              remove: true,
            },
            {
              onSuccess: () => {
                setOpen(false);
                onRemoved();
              },
            },
          )
        }
      >
        <Field label={t("lists.note")}>
          {(control) => (
            <Textarea
              {...control}
              value={note}
              maxLength={500}
              onChange={(event) => setNote(event.target.value)}
            />
          )}
        </Field>
      </ConfirmModal>
    </>
  );
}
