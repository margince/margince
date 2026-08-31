import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../api/client";
import { useCanWrite } from "../app/capability";
import { Button, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

/**
 * The Google OAuth app a mailbox connection is made through.
 *
 * This file owns the endpoint's hooks and onboarding's first-run step borrows
 * them, rather than each writing its own: how long a client SECRET lives in a
 * mutation's `variables` is not a rule worth having two answers to, and the
 * onboarding screen is the one most likely to be written in a hurry.
 *
 * The secret is never read back. `GET` answers whether one is stored and the
 * client id, which is not a secret — it travels in every authorization redirect,
 * and an operator needs to see WHICH app their installation uses to check it
 * against the Google console.
 */

export function useGoogleApp() {
  return useQuery({
    queryKey: ["installation-google-app"],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/installation/google-app",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function useSetGoogleApp() {
  const queryClient = useQueryClient();
  return useMutation({
    // Collected the moment nothing observes it, because what this mutation's
    // `variables` hold is a credential rather than a form field. The caller
    // resets it once settled for the same reason.
    gcTime: 0,
    mutationFn: async (vars: { clientId: string; clientSecret: string }) => {
      const { error } = await api.PUT("/installation/google-app", {
        body: { client_id: vars.clientId, client_secret: vars.clientSecret },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      // Both: the card's own view, and the first-run report that gates
      // onboarding on this very step.
      await queryClient.invalidateQueries({
        queryKey: ["installation-google-app"],
      });
      await queryClient.invalidateQueries({ queryKey: ["installation-setup"] });
    },
  });
}

function useRemoveGoogleApp() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE("/installation/google-app");
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ["installation-google-app"],
      });
      await queryClient.invalidateQueries({ queryKey: ["installation-setup"] });
    },
  });
}

// purposeLabel names one callback's flow. A lookup rather than a computed
// message key, because the catalog is a closed union: a template key would type
// as any string and a purpose the contract adds later would reach a reader as a
// raw key rather than as words.
function purposeLabel(purpose: string, t: ReturnType<typeof useT>): string {
  switch (purpose) {
    case "sign_in":
      return t("googleApp.redirect.sign_in");
    case "mailbox_connect":
      return t("googleApp.redirect.mailbox_connect");
    default:
      return purpose;
  }
}

// RedirectUris lists the callback URLs an operator must register on their OAuth
// client, one per purpose this deployment actually serves.
//
// The URLs come from the response and are never built here. They have one job —
// to be byte-identical to what Google receives — and a second spelling in the
// client is exactly how the two come apart. The backend derives them from the
// functions that send them, held by a fitness test in both directions.
//
// An empty list renders nothing rather than an empty heading: a deployment that
// serves no Google flow has nothing to register, and a bare heading would read
// as a list that failed to load.
function RedirectUris({
  uris,
}: Readonly<{
  uris: readonly { purpose: string; url: string }[];
}>) {
  const t = useT();
  const [copied, setCopied] = useState("");
  if (uris.length === 0) {
    return null;
  }
  return (
    <div className="stack-sm">
      <p className="t-label">{t("googleApp.redirectTitle")}</p>
      <p className="t-caption">{t("googleApp.redirectSub")}</p>
      {uris.map((uri) => {
        const purpose = purposeLabel(uri.purpose, t);
        return (
          <SettingRow
            key={uri.purpose}
            label={purpose}
            value={<code className="t-mono">{uri.url}</code>}
            control={
              <Button
                small
                onClick={() => {
                  // A denied clipboard permission is a rejected promise, and
                  // swallowing it would leave the button looking like it did
                  // nothing. The URL is on screen either way, so the honest
                  // failure is to stop claiming it was copied.
                  navigator.clipboard.writeText(uri.url).then(
                    () => setCopied(uri.purpose),
                    () => setCopied(""),
                  );
                }}
              >
                {copied === uri.purpose
                  ? t("googleApp.redirectCopied")
                  : t("googleApp.redirectCopy", { purpose })}
              </Button>
            }
          />
        );
      })}
    </div>
  );
}

export function GoogleAppCard() {
  const t = useT();
  const canManage = useCanWrite("capture_settings", "update");
  const app = useGoogleApp();
  const save = useSetGoogleApp();
  const remove = useRemoveGoogleApp();
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [confirming, setConfirming] = useState(false);
  // Focus has to land somewhere that still exists. The button that opened the
  // dialog is the natural target and the one that disappears: a successful
  // removal invalidates the query, the app reads as absent, and the Remove
  // button is gone by the time focus is restored to it.
  const clientIdRef = useRef<HTMLInputElement>(null);

  const busy = save.isPending || remove.isPending;
  const ready = clientId.trim() !== "" && clientSecret.trim() !== "";
  const failure = save.error ?? remove.error;

  return (
    <Panel title={t("googleApp.title")}>
      <PanelBody>
        <p className="t-body">{t("googleApp.sub")}</p>
        <QueryGate query={app}>
          {(status) => (
            <>
              {failure && (
                <Callout tone="danger">{problemMessageOf(failure, t)}</Callout>
              )}
              {/* Three states, because there are three answers. `configured`
                  alone could not tell "nothing anywhere" from "the deployment
                  supplies one", and the card said the former in both — telling
                  an operator Gmail could not be connected on installations
                  where it demonstrably could. */}
              <p className="t-body">
                {status.source === "stored" &&
                  t("googleApp.configured", { clientId: status.client_id })}
                {status.source === "environment" &&
                  t("googleApp.fromEnvironment", {
                    clientId: status.client_id,
                  })}
                {status.source === "none" && t("googleApp.absent")}
              </p>
              <RedirectUris uris={status.redirect_uris} />
              <Field
                label={t("firstRun.google.clientId")}
                hint={
                  status.source === "stored"
                    ? t("googleApp.replaceHint")
                    : undefined
                }
              >
                {(control) => (
                  <TextInput
                    {...control}
                    ref={clientIdRef}
                    value={clientId}
                    autoComplete="off"
                    disabled={!canManage || busy}
                    placeholder={t("firstRun.google.clientIdPlaceholder")}
                    onChange={(e) => setClientId(e.target.value)}
                  />
                )}
              </Field>
              <Field label={t("firstRun.google.clientSecret")}>
                {(control) => (
                  <TextInput
                    {...control}
                    type="password"
                    autoComplete="off"
                    value={clientSecret}
                    disabled={!canManage || busy}
                    onChange={(e) => setClientSecret(e.target.value)}
                  />
                )}
              </Field>
              <div className="row-inline">
                <Button
                  variant="primary"
                  pending={save.isPending}
                  disabled={!canManage || remove.isPending || !ready}
                  onClick={() => {
                    remove.reset();
                    save.mutate(
                      {
                        clientId: clientId.trim(),
                        clientSecret: clientSecret.trim(),
                      },
                      {
                        onSuccess: () => {
                          // The field is the only copy this app holds, and it
                          // has done its job.
                          setClientSecret("");
                          setClientId("");
                          save.reset();
                        },
                      },
                    );
                  }}
                >
                  {status.source === "stored"
                    ? t("googleApp.replace")
                    : t("googleApp.store")}
                </Button>
                {status.source === "stored" && (
                  <Button
                    variant="danger"
                    pending={remove.isPending}
                    disabled={!canManage || save.isPending}
                    onClick={() => setConfirming(true)}
                  >
                    {t("googleApp.remove")}
                  </Button>
                )}
              </div>
              <ConfirmModal
                open={confirming}
                onClose={() => setConfirming(false)}
                title={t("googleApp.removeConfirmTitle")}
                confirmLabel={t("googleApp.remove")}
                confirmVariant="danger"
                pending={remove.isPending}
                // A refused DELETE leaves this dialog OPEN, so the reason has
                // to be inside it: the background Callout is behind a modal the
                // reader cannot see past, which reads as a button that did
                // nothing.
                error={remove.error ? problemMessageOf(remove.error, t) : null}
                returnFocusTo={() => clientIdRef.current}
                onConfirm={() => {
                  save.reset();
                  remove.mutate(undefined, {
                    onSuccess: () => setConfirming(false),
                  });
                }}
              >
                {t("googleApp.removeConfirmBody")}
              </ConfirmModal>
            </>
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}
