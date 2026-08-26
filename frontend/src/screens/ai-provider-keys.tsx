import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

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

function useProviderKeys(enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: ["ai-provider-keys"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/ai/provider-keys");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// Exported for onboarding's AI step, which writes the same credential through
// the same endpoint. A second mutation there would be a second set of rules
// about how long a key lives in memory, and the ones below are not obvious
// enough to expect anybody to rediscover them.
export function useSetProviderKey() {
  const queryClient = useQueryClient();
  return useMutation({
    // Collected the moment nothing observes it, because what this mutation's
    // `variables` hold is a credential rather than a form field.
    gcTime: 0,
    // The provider AND the key travel as variables rather than closing over
    // render state: a click belongs to the render that drew it, so a value it
    // carries cannot be older than the button.
    //
    // That is also why the caller RESETS this mutation once it settles. React
    // Query keeps `variables` in the mutation's state after success, and for
    // this one mutation the variables are a credential — so what is convenient
    // for every other form is a secret held in memory, readable through the
    // observer and the devtools, until garbage collection gets to it. Passing
    // the key some other way would trade that for a stale-closure refusal,
    // which is the defect the variables rule exists to prevent, so the answer
    // is to keep the variable and drop it early.
    mutationFn: async (vars: { provider: string; apiKey: string }) => {
      const { error } = await api.PUT("/ai/provider-keys/{provider}", {
        params: { path: { provider: vars.provider } },
        body: { api_key: vars.apiKey },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["ai-provider-keys"] });
    },
  });
}

function useRemoveProviderKey() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { provider: string }) => {
      const { error } = await api.DELETE("/ai/provider-keys/{provider}", {
        params: { path: { provider: vars.provider } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["ai-provider-keys"] });
    },
  });
}

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

  if (!canSee) {
    // Withheld, not absent. An absent key card would say this installation has
    // no credentials — a claim about the DATA — where the truth is only that
    // which vendors are keyed is not this reader's to know.
    return (
      <Panel title={t("aiProviderKeys.title")}>
        <PanelBody>
          <p className="t-caption">{t("aiProviderKeys.sub")}</p>
          <EmptyState>
            <p className="t-small">{t("aiProviderKeys.withheld")}</p>
          </EmptyState>
        </PanelBody>
      </Panel>
    );
  }

  return (
    <Panel title={t("aiProviderKeys.title")}>
      <PanelBody className="form-stack">
        <p className="t-caption">{t("aiProviderKeys.sub")}</p>
        <QueryGate query={query}>
          {(list) => (
            <>
              {list.providers.map((p) => (
                <ProviderKeyRow
                  key={p.provider}
                  status={p}
                  canManage={canManage}
                />
              ))}
            </>
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

function ProviderKeyRow({
  status,
  canManage,
}: {
  status: ProviderStatus;
  canManage: boolean;
}) {
  const t = useT();
  const [value, setValue] = useState("");
  const [confirming, setConfirming] = useState(false);
  const save = useSetProviderKey();
  const remove = useRemoveProviderKey();

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

  const busy = save.isPending || remove.isPending;
  // Trimmed here as well as on the server, so the button does not offer to
  // submit a key that is only whitespace.
  const trimmed = value.trim();
  const failure = save.error ?? remove.error;

  return (
    <PanelRow>
      <Field
        label={
          <span className="row-inline">
            {status.provider}
            <Badge tone={status.configured ? "success" : "warn"} quiet>
              {status.configured
                ? t("aiProviderKeys.configured")
                : t("aiProviderKeys.absent")}
            </Badge>
          </span>
        }
        // The variable is the only thing that says HOW a key reached the
        // vault, and an operator debugging a vendor wants to know whether an
        // export seeded it.
        hint={
          status.configured
            ? t("aiProviderKeys.configuredHint", { envVar: status.env_var })
            : t("aiProviderKeys.absentHint", { envVar: status.env_var })
        }
      >
        {(control) => (
          <div className="row-inline">
            <TextInput
              {...control}
              // A password field, so the browser does not offer to remember a
              // credential this app deliberately never stores client-side, and
              // so a screenshare does not carry it.
              type="password"
              autoComplete="off"
              value={value}
              disabled={!canManage || busy}
              placeholder={
                status.configured
                  ? t("aiProviderKeys.replacePlaceholder")
                  : t("aiProviderKeys.addPlaceholder")
              }
              onChange={(e) => setValue(e.target.value)}
            />
            <Button
              variant="primary"
              // `pending` on the control that is waiting, `disabled` only for
              // the reasons it may not be pressed at all. A button carrying
              // both is natively disabled, which drops the focus and announces
              // nothing — see Button's own note on the precedence.
              pending={save.isPending}
              disabled={!canManage || remove.isPending || trimmed === ""}
              onClick={() => {
                // The other mutation's failure is no longer the current story;
                // without this its Callout stays under the row it did not come
                // from.
                remove.reset();
                save.mutate(
                  { provider: status.provider, apiKey: trimmed },
                  {
                    // Cleared on success only: a failed save leaves what was
                    // typed so it can be retried without being re-pasted.
                    onSuccess: () => setValue(""),
                  },
                );
              }}
            >
              {status.configured
                ? t("aiProviderKeys.replace")
                : t("aiProviderKeys.add")}
            </Button>
            {status.configured ? (
              <Button
                variant="danger"
                pending={remove.isPending}
                disabled={!canManage || save.isPending}
                // Confirmed first, because the act is irreversible and its cost
                // is not local to this row: the credential cannot be read back
                // to restore, and every AI lane bound to this vendor stops
                // until somebody re-pastes a key they may not have.
                onClick={() => setConfirming(true)}
              >
                {t("aiProviderKeys.remove")}
              </Button>
            ) : null}
          </div>
        )}
      </Field>
      {failure ? (
        <Callout tone="danger" live="alert">
          {problemMessageOf(failure, t)}
        </Callout>
      ) : null}
      <ConfirmModal
        open={confirming}
        onClose={() => setConfirming(false)}
        title={t("aiProviderKeys.removeConfirmTitle", {
          provider: status.provider,
        })}
        confirmLabel={t("aiProviderKeys.remove")}
        confirmVariant="danger"
        pending={remove.isPending}
        onConfirm={() => {
          save.reset();
          remove.mutate(
            { provider: status.provider },
            { onSuccess: () => setConfirming(false) },
          );
        }}
      >
        {t("aiProviderKeys.removeConfirmBody")}
      </ConfirmModal>
    </PanelRow>
  );
}
