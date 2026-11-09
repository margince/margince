// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../../api/client";
import {
  ALL_AREAS,
  areasComplete,
  type PassportAreas,
  type WorkArea,
  WorkAreasChoice,
} from "../../components/workareas";
import {
  Button,
  Checkbox,
  Field,
  Modal,
  TextInput,
} from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { WriteRefused } from "../../design-system/querystates";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { throwProblem } from "../../lib/problem";
import { resetToSignedOut } from "../../lib/session";
import { MintedPassport } from "./settings.passportuse";

const PASSPORT_SCOPES = ["read", "draft", "write", "send", "enrich"] as const;
type PassportScope = (typeof PASSPORT_SCOPES)[number];

// Annotated, so an added scope is a missing-key compile error rather than a
// checkbox labelled with its wire token.
function scopeLabelKey(scope: PassportScope): MessageKey {
  return `passport.scope.${scope}`;
}

// Why New passport cannot be pressed yet, or undefined when it can.
function mintRefusal(
  t: ReturnType<typeof useT>,
  scopeCount: number,
  areas: PassportAreas,
  pending: boolean,
): string | undefined {
  if (pending) return undefined;
  if (scopeCount === 0) return t("settings.passportScopesRequired");
  return areasComplete(areas) ? undefined : t("workArea.pickOne");
}

export function MintDialog({
  open,
  onClose,
  apiBaseUrl,
  reachable,
  onMinted,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  apiBaseUrl: string | undefined;
  reachable: readonly WorkArea[];
  onMinted: () => unknown;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [label, setLabel] = useState("");
  const [scopes, setScopes] = useState<Set<PassportScope>>(
    new Set(["read", "draft"]),
  );
  const [areas, setAreas] = useState<PassportAreas>(ALL_AREAS);
  const tokenRegion = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const formId = useId();
  const scopeHintId = useId();

  const mint = useMutation({
    mutationFn: async (request: {
      label: string;
      scopes: PassportScope[];
      areas: PassportAreas;
    }) => {
      const { data, error, response } = await api.POST("/passports", {
        body: {
          label: request.label.trim() || null,
          scopes: request.scopes,
          areas: request.areas,
        },
      });
      // An expired session must not read as a mint that did nothing. Drop every
      // cached answer before the probe re-asks, so the next sign-in sees none.
      if (response.status === 401) {
        await resetToSignedOut(queryClient);
      }
      if (error) {
        throwProblem(error);
      }
      if (!response.ok) {
        throwProblem({
          type: "about:blank",
          title: response.statusText || "Request failed",
          status: response.status,
        });
      }
      return data;
    },
    onSuccess: () => onMinted(),
  });

  // The token is shown once and never re-served, so focus goes to it.
  const minted = mint.isSuccess;
  useEffect(() => {
    if (minted) {
      tokenRegion.current?.focus();
    }
  }, [minted]);

  const close = () => {
    onClose();
    setLabel("");
    setScopes(new Set(["read", "draft"]));
    setAreas(ALL_AREAS);
    mint.reset();
  };
  // Refused mid-flight, as `mint.reset()` does not cancel the POST. Once the
  // token shows, Done is the only exit: anything else would lose it.
  const dismiss = () => {
    if (!mint.isPending && !mint.isSuccess) {
      close();
    }
  };

  return (
    <Modal
      open={open}
      onClose={dismiss}
      closeDisabled={mint.isPending || mint.isSuccess}
      labelledBy={titleId}
      intent="form"
    >
      <Heading size="large" className="t-h2 modal-title" id={titleId}>
        {t("settings.mint")}
      </Heading>
      {/* Mounted for the dialog's whole life: a live region inserted with its
          content is not reliably announced. */}
      <div className="passport-token" ref={tokenRegion} tabIndex={-1}>
        <div role="status">
          {mint.isSuccess && <p>{t("settings.passportCreated")}</p>}
        </div>
        {mint.isSuccess && (
          <MintedPassport token={mint.data.token} apiBaseUrl={apiBaseUrl} />
        )}
      </div>
      {!mint.isSuccess && (
        <form
          id={formId}
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            if (scopes.size > 0 && areasComplete(areas) && !mint.isPending) {
              mint.mutate({ label, scopes: [...scopes], areas });
            }
          }}
        >
          <Field label={t("settings.passportLabel")}>
            {(control) => (
              <TextInput
                {...control}
                value={label}
                onChange={(event) => setLabel(event.target.value)}
              />
            )}
          </Field>
          <fieldset
            className="field-multiselect"
            aria-describedby={scopeHintId}
          >
            <legend className="t-name">{t("settings.passportScopes")}</legend>
            <p id={scopeHintId} className="t-caption">
              {t("settings.passportScopesHint")}
            </p>
            {PASSPORT_SCOPES.map((scope) => (
              <Checkbox
                key={scope}
                checked={scopes.has(scope)}
                onChange={(event) => {
                  const next = new Set(scopes);
                  if (event.target.checked) {
                    next.add(scope);
                  } else {
                    next.delete(scope);
                  }
                  setScopes(next);
                }}
                label={t(scopeLabelKey(scope))}
              />
            ))}
          </fieldset>
          <WorkAreasChoice
            reachable={reachable}
            value={areas}
            onChange={setAreas}
          />
          <WriteRefused titleKey="settings.mintFailed" error={mint.error} />
        </form>
      )}
      <div className="actions">
        {mint.isSuccess ? (
          // Open on success: closing would take the one sight of the token.
          <Button variant="primary" onClick={close}>
            {t("settings.mintDone")}
          </Button>
        ) : (
          <>
            <Button disabled={mint.isPending} onClick={dismiss}>
              {t("settings.mintCancel")}
            </Button>
            <Button
              type="submit"
              form={formId}
              variant="primary"
              reason={mintRefusal(t, scopes.size, areas, mint.isPending)}
              pending={mint.isPending}
              busyLabel={t("settings.minting")}
            >
              {t("settings.mint")}
            </Button>
          </>
        )}
      </div>
    </Modal>
  );
}
