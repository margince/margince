import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { type FormEvent, useId, useState } from "react";
import { api } from "../api/client";
import { useCanWrite } from "../app/capability";
import { isOption } from "../app/options";
import { routeHash } from "../app/router";
import { Badge, Button, Field, Modal, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ChoiceList } from "../design-system/choicelist";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
import { LEAD_LIST_KEY } from "./leadkeys";
import {
  LEAD_DISQUALIFY_REASONS_KEY,
  LEAD_SOURCES_KEY,
  type LeadDisqualifyReason,
  type LeadSource,
  type LeadSourceIntent,
  sourceKeyLabel,
  useLeadDisqualifyReasons,
  useLeadSettings,
  useLeadSources,
  useUpdateLeadSettings,
} from "./leadsources";
import { settingsHref } from "./settingsrouting";
import "./leadvocab.css";
import { type VocabRename, VocabTable } from "./leadvocab.table";

// Settings › Lead handling: the two administered lead vocabularies and the
// reply deadline. Every role reads them (the leads list needs the labels and
// the deadline switch to know what to render) and only a seat holding the
// custom_field write verbs changes them. The server stays the RBAC authority;
// the controls disable with the reason rather than hide.

const INTENTS = ["high", "neutral", "low"] as const;

// The contract promises the arrays; a body that lost one is treated as the
// empty list it claims rather than a crash in the render that reads it.
function rowsOf<Row>(rows: readonly Row[] | null | undefined): readonly Row[] {
  return Array.isArray(rows) ? rows : [];
}

// The reply deadline's bounds, the same the server enforces (15 minutes to
// 7 days); checked here so a refusal never leaves the page.
const TARGET_MIN_MINUTES = 15;
const TARGET_MAX_MINUTES = 7 * 24 * 60;

// The stored intent stays high, neutral and low; the reader is told what it
// does to the score instead.
const intentLabel: Record<LeadSourceIntent, MessageKey> = {
  high: "leadVocab.score.high",
  neutral: "leadVocab.score.neutral",
  low: "leadVocab.score.low",
};
const intentDescription: Record<LeadSourceIntent, MessageKey> = {
  high: "leadVocab.score.highDesc",
  neutral: "leadVocab.score.neutralDesc",
  low: "leadVocab.score.lowDesc",
};

type SourcePatch = {
  id: string;
  label?: string;
  intent?: LeadSourceIntent;
  active?: boolean;
};

async function patchSource({ id, ...body }: SourcePatch) {
  const { data, error } = await api.PATCH("/lead-sources/{id}", {
    params: { path: { id } },
    body,
  });
  if (error) {
    throwProblem(error);
  }
  return data;
}

function useSourceMutations() {
  const queryClient = useQueryClient();
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: LEAD_SOURCES_KEY });
    // The list and the create form render labels off this list.
    void queryClient.invalidateQueries({ queryKey: LEAD_LIST_KEY });
  };
  const create = useMutation({
    mutationFn: async (body: {
      label: string;
      key?: string;
      intent: LeadSourceIntent;
    }) => {
      const { data, error } = await api.POST("/lead-sources", { body });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
  // Two instances of one write: a rename's refusal speaks in its own dialog,
  // a re-weight or a switch flip in the card, and one mutation cannot tell
  // which of the two it is reporting.
  const update = useMutation({
    mutationFn: patchSource,
    onSuccess: invalidate,
  });
  const rename = useMutation({
    mutationFn: patchSource,
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE("/lead-sources/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: invalidate,
  });
  return { create, update, rename, remove };
}

// The slice of a rename mutation the table needs, so sources and reasons
// hand theirs over through one adapter.
type RenameMutation = Readonly<{
  isPending: boolean;
  isError: boolean;
  error: unknown;
  reset: () => void;
  mutate(
    patch: { id: string; label: string },
    options: { onSuccess: () => void },
  ): void;
}>;

function renameOf(mutation: RenameMutation): VocabRename {
  return {
    pending: mutation.isPending,
    error: mutation.isError ? mutation.error : undefined,
    reset: mutation.reset,
    save: (term, label, onSaved) =>
      mutation.mutate({ id: term.id, label }, { onSuccess: onSaved }),
  };
}

// A name and a score effect, committed together, and mounted only while open
// so a half-typed source is gone the next time New source is pressed.
function AddSourceDialog({
  create,
  onClose,
}: Readonly<{
  create: ReturnType<typeof useSourceMutations>["create"];
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const [label, setLabel] = useState("");
  const [intent, setIntent] = useState<LeadSourceIntent>("neutral");
  const ready = label.trim() !== "";
  return (
    <Modal open onClose={onClose} labelledBy={titleId}>
      <form
        className="form-stack"
        onSubmit={(e) => {
          e.preventDefault();
          if (!ready || create.isPending) return;
          create.mutate(
            { label: label.trim(), intent },
            { onSuccess: onClose },
          );
        }}
      >
        <Heading size="large" className="t-h3" id={titleId}>
          {t("leadVocab.new.title")}
        </Heading>
        {create.isError && (
          <Callout
            kind="outcome"
            tone="danger"
            title={t("leadSources.notAdded")}
          >
            {problemMessageOf(create.error, t)}
          </Callout>
        )}
        <Field
          label={t("leadVocab.nameField")}
          hint={t("leadVocab.new.nameHint")}
        >
          {(control) => (
            <TextInput
              {...control}
              data-testid="lead-source-new-label"
              placeholder={t("leadSources.newPlaceholder")}
              value={label}
              onChange={(e) => setLabel(e.target.value)}
            />
          )}
        </Field>
        <div className="lead-vocab-score">
          <ChoiceList
            legend={t("leadVocab.col.score")}
            layout="cards"
            value={intent}
            onChange={(value) => {
              if (isOption(value, INTENTS)) setIntent(value);
            }}
            choices={INTENTS.map((value) => ({
              value,
              label: t(intentLabel[value]),
              description: t(intentDescription[value]),
            }))}
          />
          <p className="t-caption">{t("leadVocab.score.rescoreNote")}</p>
        </div>
        <div className="form-actions">
          <Button variant="ghost" onClick={onClose}>
            {t("deals.cancel")}
          </Button>
          {/* `!ready` is a form with nothing in it yet and `isPending` a write
              already on its way; one `disabled` covering both draws them alike. */}
          <Button
            type="submit"
            variant="primary"
            disabled={!create.isPending && !ready}
            pending={create.isPending}
          >
            {t("leadSources.add")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/**
 * The card's posture and its refused write, said once for the whole card
 * rather than on each of a dozen refused controls. All three cards carry it.
 */
function VocabNotices({
  readOnly = false,
  error,
}: Readonly<{ readOnly?: boolean; error: unknown }>) {
  const t = useT();
  if (!readOnly && error === undefined) return null;
  return (
    <div className="lead-vocab-notices">
      {readOnly && (
        <Callout kind="standing" title={t("leadSources.readOnlyTitle")} />
      )}
      {error !== undefined && (
        <Callout kind="outcome" tone="danger" title={t("leadSources.notSaved")}>
          {problemMessageOf(error, t)}
        </Callout>
      )}
    </div>
  );
}

// Values seen on leads but missing from the list: a fact about the
// integrations this installation runs, so the block is absent until one exists.
function DiscoveredSources({
  discovered,
  administered,
  canCreate,
  onAdopt,
}: Readonly<{
  discovered: readonly { key: string; lead_count: number }[];
  administered: readonly LeadSource[];
  canCreate: boolean;
  onAdopt: (key: string, label: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  type Found = (typeof discovered)[number];
  const columns: DataTableColumn<Found>[] = [
    {
      key: "name",
      header: t("leadVocab.col.source"),
      grow: true,
      render: (found) => sourceKeyLabel(found.key, administered, t),
    },
    {
      key: "leads",
      header: t("leadVocab.col.leads"),
      align: "end",
      render: (found) => formatNumber(found.lead_count, locale),
    },
    {
      key: "actions",
      header: "",
      render: (found) =>
        canCreate && (
          <Button
            onClick={() =>
              onAdopt(found.key, sourceKeyLabel(found.key, administered, t))
            }
          >
            {t("leadSources.adopt")}
          </Button>
        ),
    },
  ];
  return (
    <section
      className="lead-vocab-discovered"
      data-testid="lead-source-discovered"
    >
      <PanelBody>
        <Heading size="small" as="h3">
          {t("leadSources.discovered")}
        </Heading>
        <p className="t-caption">{t("leadSources.discoveredSub")}</p>
      </PanelBody>
      <div className="lead-vocab-table">
        <DataTable
          label={t("leadSources.discovered")}
          columns={columns}
          rows={[...discovered]}
          rowKey={(found) => found.key}
        />
      </div>
    </section>
  );
}

export function LeadSourcesCard() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  const canRemove = useCanWrite("custom_field", "delete");
  const query = useLeadSources();
  const { create, update, rename, remove } = useSourceMutations();
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<LeadSource | null>(null);
  // `remove` and `rename` speak in the dialogs that asked for them. What is
  // left are the writes with nowhere else to speak: a re-weight, a switch
  // flip, and the one-click adopt of a discovered value.
  const failure = [create, update].find((m) => m.isError);
  const administered = rowsOf(query.data?.data);
  const discovered = rowsOf(query.data?.discovered);
  const scoreColumn: DataTableColumn<LeadSource> = {
    key: "score",
    header: t("leadVocab.col.score"),
    render: (source) => (
      <Select
        aria-label={t("leadVocab.score.for", { label: source.label })}
        value={source.intent}
        disabled={!canEdit}
        onChange={(value) => {
          if (isOption(value, INTENTS) && value !== source.intent) {
            update.mutate({ id: source.id, intent: value });
          }
        }}
        options={INTENTS.map((value) => ({
          value,
          label: t(intentLabel[value]),
        }))}
      />
    ),
  };
  // Only the page's name is the link, wherever each language puts it.
  const [acquisitionBefore, acquisitionAfter = ""] = t(
    "leadVocab.sources.acquisitionLine",
  ).split("{link}");
  return (
    <Panel
      title={t("leadVocab.sources.title")}
      titleAction={
        <>
          {query.data && (
            <span className="lead-vocab-count">
              <Badge>
                {plural("leadVocab.sources.count", administered.length, {
                  count: formatNumber(administered.length, locale),
                })}
              </Badge>
            </span>
          )}
          {canCreate && (
            <Button variant="ghost" onClick={() => setAdding(true)}>
              <Plus aria-hidden /> {t("leadSources.addOpen")}
            </Button>
          )}
        </>
      }
      footer={<span className="t-caption">{t("leadVocab.sources.foot")}</span>}
    >
      <PanelBody>
        <PanelIntro>
          {t("leadVocab.sources.intro")} {acquisitionBefore}
          <a href={routeHash(settingsHref("acquisition"))}>
            {t("leadVocab.sources.acquisitionLink")}
          </a>
          {acquisitionAfter}
        </PanelIntro>
        <VocabNotices readOnly={!canEdit} error={failure?.error} />
      </PanelBody>
      {discovered.length > 0 && (
        <DiscoveredSources
          discovered={discovered}
          administered={administered}
          canCreate={canCreate}
          onAdopt={(key, label) =>
            create.mutate({ key, label, intent: "neutral" })
          }
        />
      )}
      <QueryGate query={query} pendingLabel={t("leadVocab.sources.title")}>
        {(list) => (
          <VocabTable
            label={t("leadVocab.sources.title")}
            nameHeader={t("leadVocab.col.source")}
            terms={rowsOf(list.data)}
            canEdit={canEdit}
            canRemove={canRemove}
            scoreColumn={scoreColumn}
            keptReason={(source) =>
              source.system === true
                ? t("leadVocab.sources.builtInKept")
                : plural("leadVocab.sources.inUse", source.lead_count ?? 0, {
                    count: formatNumber(source.lead_count ?? 0, locale),
                  })
            }
            renameCaption={(source) =>
              t("leadVocab.sources.keyCaption", { key: source.key })
            }
            rename={renameOf(rename)}
            onActive={(source, active) =>
              update.mutate({ id: source.id, active })
            }
            onRemove={setRemoving}
          />
        )}
      </QueryGate>
      {/* Closing clears the refusal with the form that carried it. */}
      {adding && (
        <AddSourceDialog
          create={create}
          onClose={() => {
            create.reset();
            setAdding(false);
          }}
        />
      )}
      <ConfirmModal
        open={removing !== null}
        onClose={() => {
          remove.reset();
          setRemoving(null);
        }}
        title={t("leadSources.removeTitle")}
        confirmLabel={t("leadSources.remove")}
        confirmVariant="danger"
        pending={remove.isPending}
        error={remove.isError ? problemMessageOf(remove.error, t) : null}
        onConfirm={() => {
          if (removing) {
            remove.mutate(removing.id, {
              onSuccess: () => setRemoving(null),
            });
          }
        }}
      >
        <p>{t("leadSources.removeBody", { label: removing?.label ?? "" })}</p>
      </ConfirmModal>
    </Panel>
  );
}

type ReasonPatch = { id: string; label?: string; active?: boolean };

async function patchReason({ id, ...body }: ReasonPatch) {
  const { data, error } = await api.PATCH("/lead-disqualify-reasons/{id}", {
    params: { path: { id } },
    body,
  });
  if (error) {
    throwProblem(error);
  }
  return data;
}

function useReasonMutations() {
  const queryClient = useQueryClient();
  const invalidate = () => {
    void queryClient.invalidateQueries({
      queryKey: LEAD_DISQUALIFY_REASONS_KEY,
    });
  };
  const create = useMutation({
    mutationFn: async (body: { label: string }) => {
      const { data, error } = await api.POST("/lead-disqualify-reasons", {
        body,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
  // Split for the reason useSourceMutations gives.
  const update = useMutation({
    mutationFn: patchReason,
    onSuccess: invalidate,
  });
  const rename = useMutation({
    mutationFn: patchReason,
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE("/lead-disqualify-reasons/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: invalidate,
  });
  return { create, update, rename, remove };
}

// One input and the verb that submits it, in the panel's own action band: a
// reason is a sentence and nothing else, so it needs no dialog.
function AddReasonForm({
  create,
}: Readonly<{ create: ReturnType<typeof useReasonMutations>["create"] }>) {
  const t = useT();
  const [label, setLabel] = useState("");
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (label.trim() === "") return;
    create.mutate({ label: label.trim() }, { onSuccess: () => setLabel("") });
  };
  return (
    <form className="lead-vocab-add" onSubmit={submit}>
      <TextInput
        aria-label={t("leadReasons.newLabel")}
        data-testid="lead-reason-new-label"
        placeholder={t("leadVocab.reasons.placeholder")}
        value={label}
        onChange={(e) => setLabel(e.target.value)}
      />
      <Button type="submit" variant="ghost" disabled={create.isPending}>
        {t("leadReasons.add")}
      </Button>
    </form>
  );
}

export function LeadDisqualifyReasonsCard() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  const canRemove = useCanWrite("custom_field", "delete");
  const query = useLeadDisqualifyReasons();
  const { create, update, rename, remove } = useReasonMutations();
  const [removing, setRemoving] = useState<LeadDisqualifyReason | null>(null);
  // See LeadSourcesCard: the dialogs report their own refusals.
  const failure = [create, update].find((m) => m.isError);
  const reasons = rowsOf(query.data);
  return (
    <Panel
      title={t("leadVocab.reasons.title")}
      titleAction={
        query.data && (
          <Badge>
            {plural("leadVocab.reasons.count", reasons.length, {
              count: formatNumber(reasons.length, locale),
            })}
          </Badge>
        )
      }
      actions={canCreate && <AddReasonForm create={create} />}
    >
      <PanelBody>
        <PanelIntro>{t("leadVocab.reasons.intro")}</PanelIntro>
        <VocabNotices readOnly={!canEdit} error={failure?.error} />
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("leadVocab.reasons.title")}>
        {(list) => (
          <VocabTable
            label={t("leadVocab.reasons.title")}
            nameHeader={t("leadVocab.col.reason")}
            terms={rowsOf(list)}
            canEdit={canEdit}
            canRemove={canRemove}
            keptReason={(reason) =>
              reason.system === true
                ? t("leadVocab.reasons.builtInKept")
                : plural("leadVocab.reasons.inUse", reason.lead_count ?? 0, {
                    count: formatNumber(reason.lead_count ?? 0, locale),
                  })
            }
            rename={renameOf(rename)}
            onActive={(reason, active) =>
              update.mutate({ id: reason.id, active })
            }
            onRemove={setRemoving}
          />
        )}
      </QueryGate>
      <ConfirmModal
        open={removing !== null}
        onClose={() => {
          remove.reset();
          setRemoving(null);
        }}
        title={t("leadReasons.removeTitle")}
        confirmLabel={t("leadSources.remove")}
        confirmVariant="danger"
        pending={remove.isPending}
        error={remove.isError ? problemMessageOf(remove.error, t) : null}
        onConfirm={() => {
          if (removing) {
            remove.mutate(removing.id, {
              onSuccess: () => setRemoving(null),
            });
          }
        }}
      >
        <p>{t("leadReasons.removeBody", { label: removing?.label ?? "" })}</p>
      </ConfirmModal>
    </Panel>
  );
}

// The reply deadline: off by default, and the number is the installation's
// own. The switch writes when flipped; the target commits on Enter or blur,
// only once the value is a whole number in range.
export function LeadHandlingCard() {
  const t = useT();
  const canEdit = useCanWrite("custom_field", "update");
  const query = useLeadSettings();
  const update = useUpdateLeadSettings();
  const [draft, setDraft] = useState<string | null>(null);
  const [targetError, setTargetError] = useState<string | null>(null);
  // Minted unconditionally: a hook may not depend on whether the value in the
  // box is currently refused.
  const targetErrorId = useId();
  const enabled = query.data?.first_response_enabled;
  return (
    <Panel
      title={t("leadVocab.deadline.title")}
      titleAction={
        enabled !== undefined && (
          <Badge tone={enabled ? "success" : "default"}>
            {t(enabled ? "leadVocab.deadline.on" : "leadVocab.deadline.off")}
          </Badge>
        )
      }
    >
      <PanelBody>
        <VocabNotices
          readOnly={!canEdit}
          error={update.isError ? update.error : undefined}
        />
        <QueryGate query={query} pendingLabel={t("leadVocab.deadline.title")}>
          {(settings) => {
            const shown =
              draft ?? String(settings.first_response_target_minutes);
            const commit = () => {
              const minutes = Number(shown);
              if (minutes === settings.first_response_target_minutes) {
                setDraft(null);
                setTargetError(null);
                return;
              }
              if (
                !Number.isInteger(minutes) ||
                minutes < TARGET_MIN_MINUTES ||
                minutes > TARGET_MAX_MINUTES
              ) {
                // The typed value stays so it can be corrected, and the
                // field says what it wants.
                setTargetError(t("leadHandling.targetOutOfRange"));
                return;
              }
              setTargetError(null);
              update.mutate(
                { first_response_target_minutes: minutes },
                // A refused write keeps the draft for the reader to fix; a
                // landed one clears it so the field reads the stored value.
                { onSuccess: () => setDraft(null) },
              );
            };
            return (
              <SettingList>
                <SettingRow
                  label={t("leadVocab.deadline.switch")}
                  description={t("leadVocab.deadline.switchHint")}
                  control={(control) => (
                    // The switch owns its hidden label, so it takes only the
                    // row's description. `reason` refuses the flip and says why.
                    <Switch
                      describedBy={control["aria-describedby"]}
                      label={t("leadVocab.deadline.switch")}
                      labelHidden
                      checked={settings.first_response_enabled}
                      pending={update.isPending}
                      reason={canEdit ? undefined : t("leadSources.readOnly")}
                      testId="lead-first-response-switch"
                      onChange={(next) =>
                        update.mutate({ first_response_enabled: next })
                      }
                    />
                  )}
                />
                <SettingRow
                  label={t("leadVocab.deadline.target")}
                  description={t("leadVocab.deadline.targetHint")}
                  control={(control) => (
                    <div className="settingrow-measure lead-handling-target">
                      <TextInput
                        {...control}
                        // A value out of range ADDS the refusal to the row's
                        // description, so a reader hears the rule and the miss.
                        aria-describedby={
                          [
                            control["aria-describedby"],
                            targetError === null ? null : targetErrorId,
                          ]
                            .filter(Boolean)
                            .join(" ") || undefined
                        }
                        aria-invalid={targetError === null ? undefined : true}
                        data-testid="lead-first-response-target"
                        inputMode="numeric"
                        value={shown}
                        disabled={!canEdit || !settings.first_response_enabled}
                        onChange={(e) => setDraft(e.target.value)}
                        onBlur={commit}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") {
                            e.preventDefault();
                            commit();
                          }
                        }}
                      />
                      <ErrorLine id={targetErrorId}>{targetError}</ErrorLine>
                    </div>
                  )}
                />
              </SettingList>
            );
          }}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}
