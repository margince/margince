// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../../api/schema";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import {
  ServiceAccountKeyField,
  serviceAccountProblem,
} from "../../design-system/serviceaccountkeyfield";
import { formatDateTimeAbbrev } from "../../format/format";
import type { FieldLock } from "../../format/settinglocks";
import { useLocale, useT } from "../../i18n";
import { problemMessageOf } from "../../lib/problem";
import {
  useChangeGoogleDirectory,
  useGoogleDirectory,
} from "../../lib/sign-in-policy";
import { useViewerZone } from "../../lib/viewerzone";

type DirectoryStatus = components["schemas"]["GoogleDirectoryStatus"];

/**
 * Google's ID tokens carry no groups, so the group role grants reach a Google
 * member only through this connection. The key is write-only: the panel shows
 * whether one is stored and how the newest lookup went, never the key.
 */
export function GoogleDirectoryPanel({
  canRead,
  canManage,
  lock,
}: Readonly<{ canRead: boolean; canManage: boolean; lock: FieldLock }>) {
  const t = useT();
  const status = useGoogleDirectory(canRead);
  const { store, remove } = useChangeGoogleDirectory();
  const [key, setKey] = useState("");
  const [admin, setAdmin] = useState("");
  const busy = store.isPending || remove.isPending || lock.disabled;
  const keyProblem = key === "" ? undefined : serviceAccountProblem(key);
  const refusal = store.error ?? remove.error;
  // Why Check and store cannot be pressed yet, drawn beside the button.
  let storeRefusal: string | undefined;
  if (keyProblem) {
    storeRefusal = t(keyProblem);
  } else if (key.trim() === "" || admin.trim() === "") {
    storeRefusal = t("googleDirectory.needBoth");
  }
  return (
    <Panel title={t("googleDirectory.title")}>
      <PanelBody>
        <PanelIntro>{t("googleDirectory.sub")}</PanelIntro>
        {lock.disabled && <p className="t-caption">{lock.reason}</p>}
        {status.data && <DirectoryState status={status.data} />}
        {refusal && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("signInMethods.saveFailed")}
          >
            {problemMessageOf(refusal, t)}
          </Callout>
        )}
        {canManage && (
          <>
            <ServiceAccountKeyField
              value={key}
              disabled={busy}
              error={keyProblem ? t(keyProblem) : undefined}
              onChange={setKey}
            />
            <Field
              label={t("googleDirectory.admin")}
              hint={t("googleDirectory.adminHint")}
            >
              {(control) => (
                <TextInput
                  {...control}
                  type="email"
                  autoComplete="off"
                  value={admin}
                  disabled={busy}
                  onChange={(e) => setAdmin(e.target.value)}
                />
              )}
            </Field>
            <div className="group-grants-actions">
              <Button
                variant="primary"
                pending={store.isPending}
                disabled={busy}
                reason={storeRefusal}
                onClick={() =>
                  store.mutate(
                    { service_account_json: key, admin_subject: admin },
                    {
                      onSuccess: () => {
                        setKey("");
                        setAdmin("");
                      },
                    },
                  )
                }
              >
                {t("googleDirectory.save")}
              </Button>
              {status.data?.configured && (
                <Button
                  disabled={busy}
                  pending={remove.isPending}
                  onClick={() => remove.mutate()}
                >
                  {t("googleDirectory.remove")}
                </Button>
              )}
            </div>
          </>
        )}
      </PanelBody>
    </Panel>
  );
}

// What the newest check found, in one line: a failure says what to fix.
function DirectoryState({ status }: Readonly<{ status: DirectoryStatus }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  if (!status.configured) {
    return <p>{t("googleDirectory.state.none")}</p>;
  }
  if (!status.last_checked_at || !status.last_result) {
    return <p>{t("googleDirectory.state.unchecked")}</p>;
  }
  const time = formatDateTimeAbbrev(status.last_checked_at, locale, zone);
  if (status.last_result === "ok") {
    return <p>{t("googleDirectory.state.ok", { time })}</p>;
  }
  return (
    <Callout
      tone="warning"
      kind="standing"
      title={t("googleDirectory.state.failedTitle", { time })}
    >
      <p>
        {status.last_reason
          ? t(`googleDirectory.reason.${status.last_reason}`)
          : t("googleDirectory.reason.unavailable")}
      </p>
      <p>{t("googleDirectory.state.failedEffect")}</p>
    </Callout>
  );
}
