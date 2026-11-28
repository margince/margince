// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Badge,
  Button,
  Checkbox,
  EmptyState,
  Field,
  Modal,
  TextInput,
} from "../../design-system/atoms";
import { CardBoundary } from "../../design-system/cardboundary";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { CellStrip } from "../../design-system/listtable";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryStates } from "../../design-system/querystates";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan, useCanWrite } from "../../lib/capability";
import { labelFor } from "../../lib/labelfor";
import {
  fieldCheckSentence,
  refusalSentences,
  unwrap,
} from "../../lib/problem";
import { useMe } from "../../lib/session";
import { PurposeActions } from "./privacy.purposeactions";
import "./privacy.css";

type ConsentPurpose = components["schemas"]["ConsentPurpose"];
type PurposeSendCount = components["schemas"]["ConsentPurposeSendCount"];
type PurposeClass = components["schemas"]["ConsentPurposeClass"];

type Frequency = NonNullable<ConsentPurpose["frequency"]>;

type PurposeDraft = Readonly<{
  key: string;
  // Until the admin types a key of their own, the key follows the label.
  keyTyped: boolean;
  label: string;
  requiresDoi: boolean;
  frequency: Frequency;
}>;

const EMPTY_DRAFT: PurposeDraft = {
  key: "",
  keyTyped: false,
  label: "",
  requiresDoi: false,
  frequency: "monthly",
};

// How often a purpose's mail goes out; the subscription question states it.
const FREQUENCY_LABEL: Readonly<Record<Frequency, MessageKey>> = {
  weekly: "privacy.purposeFrequency.weekly",
  monthly: "privacy.purposeFrequency.monthly",
  quarterly: "privacy.purposeFrequency.quarterly",
};

const CLASS_LABEL: Readonly<Record<PurposeClass, MessageKey>> = {
  business_correspondence: "privacy.purposeClass.businessCorrespondence",
  transactional: "privacy.purposeClass.transactional",
  marketing: "privacy.purposeClass.marketing",
  phone_outreach: "privacy.purposeClass.phoneOutreach",
};

// The window the card counts sends over; the server's own default.
const SEND_COUNT_DAYS = 30;

function isFrequency(value: string): value is Frequency {
  return Object.hasOwn(FREQUENCY_LABEL, value);
}

// The key as the server stores and compares it: trimmed, in lower case.
function purposeKey(typed: string): string {
  return serverTrim(typed.toLowerCase());
}

// What a key may hold once trimmed and lower-cased; the server's own rule,
// which frontendlimits_test.go holds equal.
const PURPOSE_KEY_PATTERN = /^[a-z0-9_]+$/;

// The key a label suggests: accents dropped, every other run of characters a
// key may not hold turned into one underscore. A run is one underscore by then,
// so the trim takes one from each end.
function keyFromLabel(label: string): string {
  return label
    .normalize("NFD")
    .replace(/\p{M}/gu, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_|_$/g, "");
}

// The characters the server's strings.Fields splits a label on and
// strings.TrimSpace trims (Go's unicode.IsSpace). JavaScript's \s and trim()
// would add U+FEFF and leave out U+0085.
const SERVER_SPACE =
  "[\\t\\n\\v\\f\\r \\u0085\\u00a0\\u1680\\u2000-\\u200a\\u2028\\u2029\\u202f\\u205f\\u3000]";
const LABEL_SPACES = new RegExp(`${SERVER_SPACE}+`);
const EDGE_SPACES = new RegExp(`^${SERVER_SPACE}+|${SERVER_SPACE}+$`, "g");

// A key or label with its ends trimmed as the server trims them.
function serverTrim(text: string): string {
  return text.replace(EDGE_SPACES, "");
}

// A label or key as a reader tells it apart: case, spacing, underscores and the
// encoding of its accents aside. Case maps up, then down with ß as ss, which
// reads as the server's purposeReading does: STRAẞE and STRASSE read as Straße.
function purposeReading(text: string): string {
  return text
    .normalize("NFC")
    .toUpperCase()
    .toLowerCase()
    .replaceAll("ß", "ss")
    .replaceAll("_", " ")
    .split(LABEL_SPACES)
    .filter((word) => word !== "")
    .join(" ");
}

// Four inputs committed together. A stale create error must not outlive the
// edit that could fix it, so every change clears it first.
function PurposeCreateForm({
  purposes,
  onDone,
}: Readonly<{ purposes: readonly ConsentPurpose[]; onDone: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<PurposeDraft>(EMPTY_DRAFT);
  const [faults, faultsRoot] = useFieldFaults();
  const formId = useId();

  const create = useMutation({
    mutationFn: async (purpose: PurposeDraft) => {
      return unwrap(
        await api.POST("/consent-purposes", {
          body: {
            key: serverTrim(purpose.key),
            label: serverTrim(purpose.label),
            requires_double_opt_in: purpose.requiresDoi,
            frequency: purpose.frequency,
          },
        }),
      );
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["consent-purposes"] });
      setDraft(EMPTY_DRAFT);
      onDone();
    },
  });

  function edit(next: Partial<PurposeDraft>) {
    setDraft((was) => {
      const merged = { ...was, ...next };
      return merged.keyTyped
        ? merged
        : { ...merged, key: keyFromLabel(merged.label) };
    });
    const fields = Object.keys(next).filter((field) => field !== "keyTyped");
    // A label that still fills the key edits the key too.
    if (next.label !== undefined && !draft.keyTyped) fields.push("key");
    for (const field of fields) faults.edited(field);
    if (create.isError) {
      create.reset();
    }
  }

  const labels = {
    key: t("privacy.purposeKey"),
    label: t("privacy.purposeLabel"),
    frequency: t("privacy.purposeFrequency"),
  };
  const holder = purposes.find(
    (purpose) => purpose.key === purposeKey(draft.key),
  );
  const reading = purposeReading(draft.label);
  const lookalike = purposes.find(
    (purpose) =>
      purposeReading(purpose.label) === reading ||
      purposeReading(purpose.key) === reading,
  );
  const conflict = {
    field: "key",
    sentence: holder
      ? t("privacy.purposeKeyTaken", { label: holder.label })
      : t("privacy.purposeKeyTakenUnnamed"),
  };
  const refused = (error: unknown) =>
    refusalSentences(error, t, labels, { conflict });

  function checked(): ReadonlyMap<string, string> {
    const checks = new Map<string, string>();
    if (serverTrim(draft.key) === "") {
      checks.set("key", fieldCheckSentence("required", locale, labels.key));
    } else if (holder) {
      checks.set("key", conflict.sentence);
    } else if (!PURPOSE_KEY_PATTERN.test(purposeKey(draft.key))) {
      checks.set("key", t("privacy.purposeKeyShape"));
    }
    if (serverTrim(draft.label) === "") {
      checks.set("label", fieldCheckSentence("required", locale, labels.label));
    } else if (lookalike) {
      checks.set(
        "label",
        t("privacy.purposeLabelTaken", { label: lookalike.label }),
      );
    }
    return checks;
  }

  const rest = refused(create.error).rest;
  return (
    <>
      <form
        id={formId}
        ref={faultsRoot}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (create.isPending || faults.show(checked())) return;
          create.mutate(draft, {
            onError: (error) => faults.show(refused(error).byField),
          });
        }}
      >
        <p>{t("privacy.purposeAppendOnly")}</p>
        <Field label={labels.label} error={faults.of("label")}>
          {(control) => (
            <TextInput
              {...control}
              value={draft.label}
              onChange={(event) => edit({ label: event.target.value })}
            />
          )}
        </Field>
        <Field
          label={labels.key}
          hint={t("privacy.purposeKeyHint")}
          error={faults.of("key")}
        >
          {(control) => (
            <TextInput
              {...control}
              value={draft.key}
              onChange={(event) =>
                edit({ key: event.target.value, keyTyped: true })
              }
            />
          )}
        </Field>
        <Field label={labels.frequency} error={faults.of("frequency")}>
          {(control) => (
            <Select
              {...control}
              options={Object.entries(FREQUENCY_LABEL).map(
                ([value, label]) => ({
                  value,
                  label: t(label),
                }),
              )}
              value={draft.frequency}
              onChange={(value) => {
                if (isFrequency(value)) edit({ frequency: value });
              }}
            />
          )}
        </Field>
        <Checkbox
          label={t("privacy.purposeDoi")}
          checked={draft.requiresDoi}
          onChange={(event) => edit({ requiresDoi: event.target.checked })}
        />
        {rest && <ErrorLine>{rest}</ErrorLine>}
      </form>
      <div className="actions">
        <Button onClick={onDone} disabled={create.isPending}>
          {t("create.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={create.isPending}
        >
          {t("privacy.purposeCreate")}
        </Button>
      </div>
    </>
  );
}

export function ConsentPurposesCard() {
  const t = useT();
  // The probe itself: every grant reads false while /me is in flight.
  const me = useMe();
  // `useCanWrite`: the seat ceiling refuses a read seat's POST before RBAC.
  const canAdminister = useCanWrite("consent_config", "create");
  const canReadSends = useCan("consent_config", "read");
  const addTitleId = useId();
  const [adding, setAdding] = useState(false);
  const query = useQuery({
    queryKey: ["consent-purposes"],
    queryFn: async () => {
      return unwrap(await api.GET("/consent-purposes"));
    },
  });
  const purposes = query.data?.data ?? [];
  return (
    <Panel
      title={t("settings.purposes")}
      titleAction={
        canAdminister ? (
          <Button aria-haspopup="dialog" onClick={() => setAdding(true)}>
            {t("privacy.addPurpose")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <PanelIntro>{t("settings.purposesSub")}</PanelIntro>
        {me.isSuccess && !canAdminister && (
          <p className="t-caption">{t("privacy.purposesReadOnly")}</p>
        )}
        {(!query.isSuccess || purposes.length === 0) && (
          <QueryStates
            query={query}
            pendingLabel={t("privacy.purposesLoading")}
          >
            <EmptyState>{t("privacy.purposesEmpty")}</EmptyState>
          </QueryStates>
        )}
      </PanelBody>
      <CardBoundary>
        {query.isSuccess && purposes.length > 0 && (
          <PurposeTable purposes={purposes} />
        )}
      </CardBoundary>
      {canReadSends && <PurposeSendCounts purposes={purposes} />}
      <Modal
        open={adding}
        onClose={() => setAdding(false)}
        labelledBy={addTitleId}
        intent="form"
      >
        <Heading size="large" id={addTitleId} className="t-h2 modal-title">
          {t("privacy.addPurpose")}
        </Heading>
        <PurposeCreateForm
          purposes={purposes}
          onDone={() => setAdding(false)}
        />
      </Modal>
    </Panel>
  );
}

function PurposeTable({ purposes }: Readonly<{ purposes: ConsentPurpose[] }>) {
  const t = useT();
  const canChange = useCanWrite("consent_config", "update");
  // The key under the label tells apart two purposes whose labels read alike.
  const columns: DataTableColumn<ConsentPurpose>[] = [
    {
      key: "purpose",
      header: t("privacy.purpose"),
      render: (purpose) => (
        <CellStack>
          <CellStrip>
            <span>{purpose.label}</span>
            {purpose.requires_double_opt_in && (
              <Badge>{t("privacy.purposeDoiBadge")}</Badge>
            )}
          </CellStrip>
          <span className="t-caption">{purpose.key}</span>
        </CellStack>
      ),
    },
  ];
  if (canChange)
    columns.push({
      key: "actions",
      header: t("table.actions"),
      render: (purpose) => <PurposeActions purpose={purpose} />,
    });
  return (
    <DataTable
      label={t("privacy.purposesRegistry")}
      bleed
      fold
      columns={columns}
      rows={purposes}
      rowKey={(purpose) => purpose.id}
    />
  );
}

// What the send engine judged each staged mail to be, per purpose. The
// disputed count is where a mail sent under the wrong kind shows afterwards.
function PurposeSendCounts({
  purposes,
}: Readonly<{ purposes: readonly ConsentPurpose[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const query = useQuery({
    queryKey: ["consent-purposes", "send-counts", SEND_COUNT_DAYS],
    queryFn: async () => {
      return unwrap(
        await api.GET("/consent-purposes/send-counts", {
          params: { query: { days: SEND_COUNT_DAYS } },
        }),
      );
    },
  });
  const counts = query.data?.data ?? [];
  const labelOf = (count: PurposeSendCount) =>
    count.class
      ? (purposes.find((purpose) => purpose.key === count.purpose_key)?.label ??
        count.purpose_key)
      : t("privacy.purposeUnknown");
  const columns: DataTableColumn<PurposeSendCount>[] = [
    {
      key: "purpose",
      header: t("privacy.purpose"),
      render: labelOf,
    },
    {
      key: "class",
      header: t("privacy.purposeClass"),
      render: (count) =>
        t(
          count.class
            ? labelFor(CLASS_LABEL, count.class)
            : "privacy.purposeClass.undefined",
        ),
    },
    {
      key: "sends",
      header: t("privacy.purposeSends", {
        days: formatNumber(SEND_COUNT_DAYS, locale),
      }),
      align: "end",
      render: (count) => formatNumber(count.sends, locale),
    },
    {
      key: "disputed",
      header: t("privacy.purposeDisputed"),
      align: "end",
      render: (count) => formatNumber(count.disputed, locale),
    },
  ];
  return (
    <>
      <PanelBody>
        <PanelIntro>
          {t("privacy.purposeSendsSub", {
            days: formatNumber(SEND_COUNT_DAYS, locale),
          })}
        </PanelIntro>
        {(!query.isSuccess || counts.length === 0) && (
          <QueryStates
            query={query}
            pendingLabel={t("privacy.purposeSendsLoading")}
          >
            <EmptyState>
              {t("privacy.purposeSendsEmpty", {
                days: formatNumber(SEND_COUNT_DAYS, locale),
              })}
            </EmptyState>
          </QueryStates>
        )}
      </PanelBody>
      <CardBoundary>
        {query.isSuccess && counts.length > 0 && (
          <DataTable
            label={t("privacy.purposeSendsTable")}
            bleed
            fold
            columns={columns}
            rows={counts}
            rowKey={(count) => count.purpose_key}
          />
        )}
      </CardBoundary>
    </>
  );
}
