import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Button,
  EmptyState,
  Field,
  Modal,
  OverflowMenu,
  SegmentedControl,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { describedByIds } from "../design-system/describedby";
import { Heading } from "../design-system/heading";
import { KeyedName } from "../design-system/keyedname";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import { throwProblem, unwrap } from "../lib/problem";
import {
  type MutationOutcome,
  useRowHandoff,
  useUndoableRemoval,
} from "../lib/undoableremoval";
import { useFolderOptions } from "./capture-exclusions.queries";
import { PurgeDialog } from "./capture-purge-dialog";
import { captureValueMessage } from "./capturevalue";
import "./capture-exclusions.css";

// Pre-capture exclusions: the addresses and domains whose mail the CRM must not
// store at all. Two scopes on one card, because a reader sees both kinds of
// rule that bind their mailbox: the company's (admin/ops change those) and
// their own (anyone may keep their own correspondent out of a shared CRM).

type CaptureExclusion = components["schemas"]["CaptureExclusion"];
type Scope = components["schemas"]["CaptureExclusionScope"];
type Kind = components["schemas"]["CaptureExclusionKind"];

const SCOPES: readonly Scope[] = ["user", "workspace"];
// The kinds this card offers. `container` is a PICKER rather than a text box:
// a container rule stores a provider's own token — a Gmail label id, a Graph
// folder id — so the list comes from the mailbox and the reader chooses a name.
const KINDS: readonly Kind[] = ["address", "domain", "container"];

/** The company-wide rules, which are the ones a plain seat may not touch. */
function bindsEveryone(rule: CaptureExclusion): boolean {
  return rule.scope === "workspace";
}

function useExclusions() {
  return useQuery({
    queryKey: ["capture-exclusions"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/capture/exclusions");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

type ExclusionBody = { scope: Scope; kind: Kind; value: string };

// Without an outcome it is the dialog's add. With one it is an Undo putting a
// removed rule back, which says so in its own words.
function useAddExclusion(outcome?: MutationOutcome<ExclusionBody>) {
  const toast = useToast();
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: ExclusionBody) => {
      return unwrap(await api.POST("/capture/exclusions", { body }));
    },
    onError: outcome?.onError,
    onSuccess: (_written, added) => {
      queryClient.invalidateQueries({ queryKey: ["capture-exclusions"] });
      if (outcome) {
        outcome.onSuccess(added);
        return;
      }
      toast.show(t("settings.addedItem", { name: added.value }));
    },
  });
}

function useRemoveExclusion(outcome: MutationOutcome<ExclusionBody | null>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (rule: CaptureExclusion): Promise<ExclusionBody> => {
      unwrap(
        await api.DELETE("/capture/exclusions/{id}", {
          params: { path: { id: rule.id } },
        }),
      );
      return { scope: rule.scope, kind: rule.kind, value: rule.value };
    },
    onError: outcome.onError,
    onSuccess: (removed) => {
      queryClient.invalidateQueries({ queryKey: ["capture-exclusions"] });
      outcome.onSuccess(removed);
    },
  });
}

/** What each scope and kind is called, in the reader's language. */
function useRuleWords() {
  const t = useT();
  const scope: Record<Scope, string> = {
    user: t("captureExclusions.scope.user"),
    workspace: t("captureExclusions.scope.workspace"),
  };
  const kind: Record<Kind, string> = {
    address: t("captureExclusions.kind.address"),
    domain: t("captureExclusions.kind.domain"),
    container: t("captureExclusions.kind.container"),
  };
  return { scope, kind };
}

export function CaptureExclusionsCard() {
  const t = useT();
  const canManageWorkspace = useCanWrite("capture_settings", "update");
  const query = useExclusions();
  const [excluding, setExcluding] = useState(false);
  const addVerb = useRef<HTMLButtonElement>(null);
  // The rule whose already-captured mail is being destroyed, held by id AND
  // value so the dialog can name what it is about to act on.
  const [purging, setPurging] = useState<{ id: string; value: string } | null>(
    null,
  );
  // Said once and pointed at (see own-domains.tsx): a company-wide rule
  // is admin/ops work, and `Button`'s `reasonId` refuses the verb AND names
  // the sentence, so every refused row points at one line rather than
  // printing it per row. The id is minted unconditionally, because a hook may
  // not depend on a permission.
  const denialId = useId();
  const rules = query.data?.data ?? [];
  // Named only when something on this card is actually refused: a standing
  // sentence about a permission with nothing behind it is a claim about the
  // reader that no row on the page bears out.
  const refusesARow =
    !canManageWorkspace && rules.some((rule) => bindsEveryone(rule));

  return (
    <Panel
      title={t("captureExclusions.title")}
      // Three inputs submitted together — who it binds, what kind of rule it
      // is, and the value — so the form lives behind this verb. It rides in the
      // header rather than in a row of its own: a row states a setting and its
      // answer, and a row whose label repeated the button's words said the same
      // thing twice a hand apart. No refusal on it, because anybody may keep
      // their OWN correspondent out — the dialog refuses the scope that binds
      // everyone, where that choice is made.
      titleAction={
        <Button ref={addVerb} onClick={() => setExcluding(true)}>
          {t("captureExclusions.addOpen")}
        </Button>
      }
    >
      <QueryGate query={query} pendingLabel={t("captureExclusions.title")}>
        {(list) => (
          <ExclusionRows
            list={list.data}
            canManageWorkspace={canManageWorkspace}
            denialId={denialId}
            landing={() => addVerb.current}
            onPurge={(id, value) => setPurging({ id, value })}
          />
        )}
      </QueryGate>
      {refusesARow && (
        <PanelBody>
          <p id={denialId}>{t("captureSettings.adminOnly")}</p>
        </PanelBody>
      )}
      {excluding && (
        <ExcludeDialog
          canManageWorkspace={canManageWorkspace}
          onClose={() => setExcluding(false)}
        />
      )}
      {purging && (
        <PurgeDialog
          ruleId={purging.id}
          ruleValue={purging.value}
          onClose={() => setPurging(null)}
        />
      )}
    </Panel>
  );
}

/** One rule per row: what it names, who it binds and its kind under it. */
function ExclusionRows({
  list,
  canManageWorkspace,
  denialId,
  landing,
  onPurge,
}: Readonly<{
  list: CaptureExclusion[];
  canManageWorkspace: boolean;
  /** The one sentence saying why a company-wide rule is not this reader's. */
  denialId: string;
  landing: () => HTMLElement | null;
  onPurge: (id: string, value: string) => void;
}>) {
  const t = useT();
  if (list.length === 0) {
    // `empty`, and only `empty`: nothing is excluded, which is a fact about the
    // installation rather than a read that failed.
    return (
      <PanelBody>
        <PanelIntro>{t("captureExclusions.sub")}</PanelIntro>
        <EmptyState>
          <p data-testid="capture-exclusions-empty">
            {t("captureExclusions.empty")}
          </p>
        </EmptyState>
      </PanelBody>
    );
  }
  return (
    <>
      <PanelBody>
        <PanelIntro>{t("captureExclusions.sub")}</PanelIntro>
      </PanelBody>
      <SettingList bleed="records" testId="capture-exclusions-list">
        {list.map((rule) => (
          <ExclusionRow
            key={rule.id}
            rule={rule}
            refusal={
              bindsEveryone(rule) && !canManageWorkspace ? denialId : undefined
            }
            landing={landing}
            onPurge={() => onPurge(rule.id, rule.value)}
          />
        ))}
      </SettingList>
    </>
  );
}

// Removing the rule stops future capture and comes back with Undo; deleting
// what it already matched cannot be undone, so that verb keeps its dialog.
function ExclusionRow({
  rule,
  refusal,
  landing,
  onPurge,
}: Readonly<{
  rule: CaptureExclusion;
  refusal: string | undefined;
  landing: () => HTMLElement | null;
  onPurge: () => void;
}>) {
  const t = useT();
  const verbs = useRowHandoff(landing);
  const words = useRuleWords();
  const toasts = useUndoableRemoval<ExclusionBody>({
    removed: t("settings.removedItem", { name: rule.value }),
    restored: t("settings.addedItem", { name: rule.value }),
  });
  const restore = useAddExclusion(toasts.restored);
  const remove = useRemoveExclusion(
    toasts.removed((body) => restore.mutate(body)),
  );
  return (
    <SettingRow
      label={
        rule.kind === "container" ? (
          <KeyedName name={rule.value} code={rule.value} />
        ) : (
          <span className="capture-exclusions-value">{rule.value}</span>
        )
      }
      description={[
        words.scope[rule.scope],
        words.kind[rule.kind],
        rule.created_by_name &&
          t("captureExclusions.addedBy", { name: rule.created_by_name }),
      ]
        .filter(Boolean)
        .join(" · ")}
      control={
        <span className="cell-actions" ref={verbs}>
          <OverflowMenu label={t("table.rowActions", { name: rule.value })}>
            <Button
              disabled={remove.isPending}
              reasonId={refusal}
              onClick={() => remove.mutate(rule)}
            >
              {t("captureExclusions.removeRule")}
            </Button>
            <Button aria-haspopup="dialog" reasonId={refusal} onClick={onPurge}>
              {t("capturePurge.openShort")}
            </Button>
          </OverflowMenu>
        </span>
      }
    />
  );
}

// Mounted only while it is open, so a half-typed address is gone the next time
// the dialog opens rather than waiting there under a scope nobody re-chose.
function ExcludeDialog({
  canManageWorkspace,
  onClose,
}: Readonly<{ canManageWorkspace: boolean; onClose: () => void }>) {
  const t = useT();
  const words = useRuleWords();
  const add = useAddExclusion();
  const headingId = useId();
  const denialId = useId();
  const formId = useId();
  const [scope, setScope] = useState<Scope>("user");
  const [kind, setKind] = useState<Kind>("address");
  const [draft, setDraft] = useState("");
  const picking = kind === "container";
  const folders = useFolderOptions(picking);
  // A container rule lives in ONE mailbox, which the database says too
  // (capture_exclusion_container_is_personal). So choosing it narrows the scope
  // rather than leaving a workspace rule that names a label meaning nothing in
  // anybody else's mail.
  const effectiveScope: Scope = kind === "container" ? "user" : scope;
  // Anyone may keep their own correspondent out of a shared CRM; a rule that
  // binds everybody is admin/ops work, and the sentence sits with the controls
  // it refuses. It follows the scope the rule would actually be WRITTEN at: a
  // container rule is forced to `user` above, so picking one after having
  // chosen workspace must not leave the picker disabled for a rule that binds
  // only the reader's own mailbox.
  const refused = effectiveScope === "workspace" && !canManageWorkspace;
  const value = draft.trim();
  return (
    <Modal open onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("captureExclusions.addLabel")}
      </Heading>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (refused || value === "") {
            return;
          }
          add.mutate(
            { scope: effectiveScope, kind, value },
            { onSuccess: onClose },
          );
        }}
      >
        {kind !== "container" && (
          <SegmentedControl
            options={SCOPES}
            value={scope}
            onChange={setScope}
            labels={words.scope}
            label={t("captureExclusions.scopeLabel")}
          />
        )}
        <SegmentedControl
          options={KINDS}
          value={kind}
          // The text box and the folder picker share one draft, and a token
          // typed as an address means nothing as a container. Carrying it
          // across would pass the submit guard and fail at the store.
          onChange={(next) => {
            setKind(next);
            setDraft("");
          }}
          labels={words.kind}
          label={t("captureExclusions.kindLabel")}
        />
        {kind === "container" ? (
          <Field
            label={t("captureExclusions.containerLabel")}
            hint={t("captureExclusions.notRetroactive")}
          >
            {(control) => (
              <Select
                {...control}
                value={draft}
                disabled={refused || folders.options.length === 0}
                options={folders.options}
                onChange={setDraft}
              />
            )}
          </Field>
        ) : (
          <Field
            label={t("captureExclusions.addLabel")}
            labelHidden
            hint={t("captureExclusions.notRetroactive")}
          >
            {(control) => (
              <TextInput
                {...control}
                value={draft}
                placeholder={
                  kind === "address"
                    ? t("captureExclusions.placeholder.address")
                    : t("captureExclusions.placeholder.domain")
                }
                disabled={refused}
                aria-describedby={describedByIds(
                  control["aria-describedby"],
                  refused && denialId,
                )}
                onChange={(event) => setDraft(event.target.value)}
              />
            )}
          </Field>
        )}
        {picking && !folders.isPending && folders.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("captureExclusions.containersUnreadable")}
          >
            {t("captureExclusions.containersUnreadableBody")}
          </Callout>
        )}
        {picking &&
          !folders.isPending &&
          !folders.isError &&
          folders.options.length === 0 && (
            <p>{t("captureExclusions.noContainers")}</p>
          )}
        {/* A list that stopped short is still worth showing, and saying so is
            what keeps a missing folder from reading as a folder that does not
            exist. */}
        {picking &&
          !folders.isPending &&
          !folders.isError &&
          folders.truncated && (
            <Callout
              tone="warning"
              kind="outcome"
              title={t("captureExclusions.containersPartial")}
            >
              {t("captureExclusions.containersPartialBody")}
            </Callout>
          )}
        {refused && <p id={denialId}>{t("captureSettings.adminOnly")}</p>}
        {add.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("captureSettings.addFailed")}
          >
            {captureValueMessage(add.error, add.variables?.kind, t)}
          </Callout>
        )}
      </form>
      <div className="actions">
        <Button type="button" onClick={onClose}>
          {t("create.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={add.isPending || value === ""}
          reasonId={refused ? denialId : undefined}
        >
          {t("captureExclusions.add")}
        </Button>
      </div>
    </Modal>
  );
}
