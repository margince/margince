import { useEffect, useState } from "react";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import { serviceAccountProblem } from "../design-system/serviceaccountkeyfield";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { ProviderRecentCalls } from "./ai-call-figures";
import { useProviderHealth } from "./ai-provider-health";
import {
  credentialKindOf,
  KeyEntry,
  keyStateLabel,
  keyStateTone,
} from "./ai-provider-key-entry";
import {
  useProviderKeys,
  useRemoveProviderKey,
  useSetProviderKey,
} from "./ai-provider-key-hooks";
import {
  KeyTestButton,
  KeyTestOutcome,
  useTestProviderKey,
} from "./ai-provider-key-test";
import { isOpenRouter } from "./ai-provider-links";
import { providerName } from "./ai-provider-names";
import {
  hasProviderSettings,
  ProviderSettingsForm,
} from "./ai-provider-settings";
import { ProviderSheet } from "./ai-provider-sheet";
import { ProviderTable } from "./ai-provider-table";
import { providerUsage, useRouting } from "./ai-routing-query";
import { PanelTitle } from "./ai-terms";
import { problemMessageOf, QueryGate } from "./common";
import "./ai-settings.css";

// The vendor credentials this installation calls models with.
//
// One rule shapes the whole card: there is no read path for a key. The server
// answers `configured` and nothing else, so this screen can offer "add" or
// "replace" and can never show, prefill, or mask an existing value. A masked
// value would be worse than none — it implies the real one is retrievable, and
// invites someone to screenshot it.
//
// Admin/ops only, on the same `ai_routing` grant the binding carries: a seat
// that may not re-point a model may not reach the credential that model would
// call with. A reader without the grant never gets here, and the form is
// disabled rather than hidden for one who can look but not change — the same
// shape the routing card uses.

type ProviderStatus = components["schemas"]["AiProviderKeyStatus"];

export function AiProviderKeysCard() {
  const t = useT();
  // Two grants, two questions. `read` decides whether the list is this reader's
  // to see at all; `update` decides whether the form is theirs to use. Asking
  // the server without the read grant would draw a 403 error box, which reads
  // as a fault in the installation rather than as a permission — and this page
  // is explicit that every card answers a denial the same way, because three
  // answers to one denial on one page is what made it unreadable.
  const canSee = useCan("ai_routing", "read");
  const canManage = useCanWrite("ai_routing", "update");
  const query = useProviderKeys(canSee);
  const routing = useRouting(canSee);
  // A separate grant from the list's: health is a diagnostic, so a reader who
  // may see the keys but not diagnostics gets the rows without the notice.
  const canDiagnose = useCan("ai_diagnostics", "read");
  // Gated again at the read: a revoked grant disables the query but leaves its
  // cached answer, which would keep drawing a diagnostic the reader may no
  // longer see.
  const health = useProviderHealth(canDiagnose).data;
  const usage = routing.data ? providerUsage(routing.data.routing) : null;
  // The provider whose sheet is open, by name so it follows the list as a key
  // is saved rather than holding a copy that goes stale.
  const [opened, setOpened] = useState<string | null>(null);
  // The host the open sheet's form would save; null until it reports one.
  const [draftHost, setDraftHost] = useState<string | null>(null);

  if (!canSee) {
    // Withheld, not absent. An absent key card would say this installation has
    // no credentials — a claim about the DATA — where the truth is only that
    // which vendors are keyed is not this reader's to know.
    return (
      <Panel
        title={
          <PanelTitle term="provider">{t("aiProviderKeys.title")}</PanelTitle>
        }
      >
        <PanelBody>
          <EmptyState>{t("aiProviderKeys.withheld")}</EmptyState>
        </PanelBody>
      </Panel>
    );
  }

  // Rows rather than a stack of forms. Every vendor asks the same three-part
  // question — who, whether it is keyed, and the way to change that — and a
  // reader auditing the page travels one column instead of reading six open
  // paste fields to find the one vendor that is not set up.
  return (
    <Panel
      title={
        <PanelTitle term="provider">{t("aiProviderKeys.title")}</PanelTitle>
      }
    >
      <QueryGate query={query} pendingLabel={t("aiProviderKeys.title")}>
        {(list) => {
          const openStatus = list.providers.find((p) => p.provider === opened);
          return (
            <>
              <ProviderTable
                providers={list.providers}
                canManage={canManage}
                usage={usage}
                health={canDiagnose ? health?.providers : undefined}
                onOpen={(provider) => {
                  setDraftHost(null);
                  setOpened(provider);
                  // The list stays mounted while sheets open and close, so
                  // a key set elsewhere shows only if opening reads again.
                  query.refetch();
                }}
              />
              {openStatus ? (
                <ProviderSheet
                  status={openStatus}
                  usage={usage?.get(openStatus.provider)}
                  health={
                    canDiagnose
                      ? health?.providers.find(
                          (h) => h.provider === openStatus.provider,
                        )
                      : undefined
                  }
                  connection={
                    <>
                      <ProviderConnection
                        status={openStatus}
                        canManage={canManage}
                      />
                      {/* Drawn once the stored settings are read: the form
                          starts from them, and one started empty would save
                          an empty entry over what is stored. */}
                      {hasProviderSettings(openStatus.provider) &&
                        routing.data && (
                          <ProviderSettingsForm
                            key={openStatus.provider}
                            provider={openStatus.provider}
                            routing={routing.data.routing}
                            canManage={canManage}
                            onHostChange={setDraftHost}
                          />
                        )}
                    </>
                  }
                  figures={
                    <ProviderRecentCalls
                      provider={openStatus.provider}
                      broker={
                        openStatus.provider === "openai_compatible" &&
                        isOpenRouter(
                          draftHost ??
                            routing.data?.routing.providers?.[
                              openStatus.provider
                            ]?.base_url ??
                            "",
                        )
                      }
                    />
                  }
                  onClose={() => {
                    setDraftHost(null);
                    setOpened(null);
                  }}
                />
              ) : null}
            </>
          );
        }}
      </QueryGate>
    </Panel>
  );
}

// What the paste field says about the key it holds. A Vertex key is only as good
// as its account's role, which no paste can show, so its hint names the role.
function keyEntryHint(
  status: ProviderStatus,
  t: ReturnType<typeof useT>,
): string {
  const stored = status.configured
    ? t("aiProviderKeys.configuredHint", { envVar: status.env_var })
    : t("aiProviderKeys.absentHint", { envVar: status.env_var });
  return status.provider === "gemini_vertex"
    ? `${stored} ${t("aiProviderKeys.vertexRoleHint")}`
    : stored;
}

// The credential controls for ONE vendor, drawn inside its sheet: whether it is
// keyed, the test, and the paste field that adds, replaces or removes the key.
function ProviderConnection({
  status,
  canManage,
}: {
  status: ProviderStatus;
  canManage: boolean;
}) {
  const t = useT();
  const [value, setValue] = useState("");
  const [confirming, setConfirming] = useState(false);
  // Whether this row's paste field is open. Folded by default: the row is a
  // READING of whether the vendor is keyed, and six open password boxes make a
  // page nobody can audit at a glance.
  const [editing, setEditing] = useState(false);
  // Set by a Save press on a service-account key the browser can already tell
  // is not one; cleared as soon as the reader edits it.
  const [refusal, setRefusal] = useState<MessageKey | undefined>();
  const kind = credentialKindOf(status);
  const save = useSetProviderKey();
  const remove = useRemoveProviderKey();
  const test = useTestProviderKey();

  // The credential leaves React Query's memory as soon as the save settles.
  //
  // `variables` are retained after success — ordinarily a convenience, and for
  // this one mutation a secret readable through the observer and the devtools
  // until garbage collection. It cannot be dropped from the per-call onSuccess:
  // the state is finalized after those callbacks run, so a reset there is
  // overwritten. An effect on the settled flag is the first point that sticks.
  //
  // Success only. A failed save keeps its error on screen, and the key the user
  // is about to retry is in the field anyway.
  useEffect(() => {
    if (save.isSuccess) {
      save.reset();
    }
  }, [save.isSuccess, save.reset, save]);

  // A test result describes the key that was held when it ran. Once that key
  // is replaced or removed the result is about nothing on screen.
  const { reset: resetTest } = test;
  useEffect(() => {
    if (save.isSuccess || remove.isSuccess) {
      resetTest();
    }
  }, [save.isSuccess, remove.isSuccess, resetTest]);

  const busy = save.isPending || remove.isPending;
  // Trimmed here as well as on the server, so the button does not offer to
  // submit a key that is only whitespace.
  const trimmed = value.trim();
  const failure = save.error ?? remove.error;
  // A vendor with no variable name is one this build reaches without a
  // credential at all. Nothing to add, nothing to replace, and saying "not set"
  // about it would report a gap that is not one.
  const keyless = status.env_var === "";

  const entryHint = keyEntryHint(status, t);
  // Save and Remove, the same pair whichever field holds the secret.
  const verbs = (
    <>
      <Button
        variant="primary"
        // `pending` on the control that is waiting, `disabled` only
        // for the reasons it may not be pressed at all. A button
        // carrying both is natively disabled, which drops the focus
        // and announces nothing — see Button's own note on the
        // precedence.
        pending={save.isPending}
        disabled={!canManage || remove.isPending || trimmed === ""}
        onClick={() => {
          // The other mutation's failure is no longer the current
          // story; without this its Callout stays under the row it
          // did not come from.
          remove.reset();
          const problem =
            kind === "service_account"
              ? serviceAccountProblem(trimmed)
              : undefined;
          if (problem) {
            save.reset();
            setRefusal(problem);
            return;
          }
          save.mutate(
            { provider: status.provider, kind, secret: trimmed },
            {
              // Cleared on success only: a failed save leaves what
              // was typed so it can be retried without being
              // re-pasted. The row folds shut on the same success,
              // because the question it was opened to answer has
              // been answered.
              onSuccess: () => {
                setValue("");
                setEditing(false);
              },
            },
          );
        }}
      >
        {t("aiProviderKeys.save")}
      </Button>
      {status.configured ? (
        <Button
          variant="danger"
          pending={remove.isPending}
          disabled={!canManage || save.isPending}
          // Confirmed first, because the act is irreversible and its
          // cost is not local to this row: the credential cannot be
          // read back to restore, and every AI lane bound to this
          // vendor stops until somebody re-pastes a key they may not
          // have.
          onClick={() => setConfirming(true)}
        >
          {t("aiProviderKeys.remove")}
        </Button>
      ) : null}
    </>
  );

  return (
    <div>
      <div
        className="form-stack"
        data-testid={`ai-provider-key-${status.provider}`}
      >
        <div className="ai-provider">
          <span className="ai-provider-who">
            {/* The variable is the only thing that says HOW a key reached the
                vault, and an operator debugging a vendor wants to know whether
                an export seeded it. Verbatim, because it is a name to be typed
                somewhere else exactly as it reads here. */}
            <span className="ai-provider-env">
              {keyless ? "\u2014" : status.env_var}
            </span>
          </span>
          <Badge tone={keyStateTone(status, keyless)}>
            {t(keyStateLabel(status, keyless, kind))}
          </Badge>
          {!keyless && (
            <span className="ai-lane-open">
              {(status.configured || status.optional) && (
                <KeyTestButton provider={status.provider} test={test} />
              )}
              <Button
                // Closing DROPS what was typed. The field holds a credential,
                // and one left in state comes back the next time the row is
                // opened — on a screenshare, or for whoever is at the desk next.
                onClick={() => {
                  if (editing) {
                    setValue("");
                    setRefusal(undefined);
                  }
                  setEditing((open) => !open);
                }}
                aria-expanded={editing}
                reason={canManage ? undefined : t("aiProviderKeys.adminOnly")}
              >
                {status.configured
                  ? t("aiProviderKeys.replace")
                  : t("aiProviderKeys.add")}
              </Button>
            </span>
          )}
        </div>
        <KeyTestOutcome test={test} keyHeld={status.configured} />
        {editing && (
          <KeyEntry
            kind={kind}
            configured={status.configured}
            value={value}
            disabled={!canManage || busy}
            hint={entryHint}
            refusal={refusal}
            onChange={(next) => {
              setRefusal(undefined);
              setValue(next);
            }}
            verbs={verbs}
          />
        )}
        {failure ? (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("aiProviderKeys.saveFailed")}
          >
            {problemMessageOf(failure, t)}
          </Callout>
        ) : null}
      </div>
      <ConfirmModal
        open={confirming}
        onClose={() => setConfirming(false)}
        title={t("aiProviderKeys.removeConfirmTitle", {
          provider: providerName(status.provider, t),
        })}
        confirmLabel={t("aiProviderKeys.remove")}
        confirmVariant="danger"
        pending={remove.isPending}
        onConfirm={() => {
          save.reset();
          remove.mutate(
            { provider: status.provider },
            {
              onSuccess: () => {
                setConfirming(false);
                setEditing(false);
                // And the draft with it. A replacement typed before the
                // operator decided to REMOVE instead would otherwise come back
                // the next time the row is opened — a key they chose to be rid
                // of, one press from being submitted again.
                setValue("");
              },
            },
          );
        }}
      >
        {t("aiProviderKeys.removeConfirmBody")}
      </ConfirmModal>
    </div>
  );
}
