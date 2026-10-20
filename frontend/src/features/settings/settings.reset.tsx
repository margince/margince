import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { operations } from "../../api/schema";
import { Button, TextInput } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { Panel, PanelBody } from "../../design-system/panel";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";

// The full reset response, derived from the generated operation type (T6: no
// `as`, no hand-typed field list). A wire change that adds or renames a counter
// fails typecheck here instead of going unshown.
type ResetSummary =
  operations["resetData"]["responses"][200]["content"]["application/json"];

// The danger-zone reset action wipes the installation back to its first-boot
// state. It needs two conditions of different kinds. The grant
// `system_reset:delete` says this reader may wipe an installation that permits
// wiping. The server-driven `data_reset_available` flag on /me says whether this
// one does; its compiled default is false in every posture.
//
// The flag is not VITE_UI_PREVIEW_RESET, which is the unrelated password-reset
// link. The server gates the endpoint on the same flag and answers 404 without it.
//
// The company's name is not on MeResponse, so this never compares it client-side.
// A non-empty input enables the confirm button, and the server judges the match
// (a mismatch comes back as a 422, shown verbatim in the dialog).
export function ResetDataCard() {
  const t = useT();
  const { locale } = useLocale();
  const me = useMe();
  // `system_reset:delete` is what POST /admin/reset-data asks for
  // (compose/datareset.go). A role edited to carry the verb reaches the control,
  // and one that lost it does not; the role name could not say either way.
  //
  // `useCanWrite`: the reset is a POST, and a read seat is refused every
  // mutation by the seat ceiling above RBAC (identity/admission.go) whatever
  // its grants say. Offering the danger zone to one would be a button that
  // types the workspace name and then 403s.
  const canSee = useCanWrite("system_reset", "delete");
  const workspaceName = me.data?.workspace_name ?? "";
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");
  // What the last reset cleared. It is null until one has run, so the danger
  // zone stays quiet on first render instead of implying a result nobody triggered.
  const [summary, setSummary] = useState<ResetSummary | null>(null);
  const queryClient = useQueryClient();

  const reset = useMutation({
    mutationFn: async () => {
      // The summary always describes the latest attempt, never a prior one.
      // A retry's error cannot leave a previous success on screen, and an
      // in-flight retry shows no summary.
      setSummary(null);
      return unwrap(
        await api.POST("/admin/reset-data", {
          body: { confirmation: typed },
        }),
      );
    },
    onSuccess: (data) => {
      setOpen(false);
      setTyped("");
      setSummary(data ?? null);
      // A reset wipes every domain table for the workspace, so every cached
      // list/detail query is stale, not only the ones this card knows about.
      queryClient.invalidateQueries();
    },
  });

  if (!canSee || !me.data?.data_reset_available) {
    return null;
  }

  return (
    <Panel
      title={t("settings.dangerZone")}
      // The one card that announces its own danger: the red border is what
      // separates a destructive surface from the ordinary settings around it.
      className="settings-danger"
    >
      <SettingList bleed="settings">
        {/* This button and the dialog's confirm share the screen, so each is
            named for its own act. */}
        <SettingRow
          label={t("settings.resetDataLabel")}
          description={t("settings.resetDataDesc")}
          control={
            <Button variant="danger" onClick={() => setOpen(true)}>
              {t("settings.resetDataButton")}
            </Button>
          }
        />
      </SettingList>
      {summary && (
        <PanelBody>
          <p className="t-caption settings-danger-result" role="status">
            {t("settings.resetDataResult", {
              tables: formatNumber(summary.tables_cleared, locale),
              jobs: formatNumber(summary.jobs_deleted, locale),
              streams: formatNumber(summary.streams_purged, locale),
              keys: formatNumber(summary.cache_keys_deleted, locale),
              objects: formatNumber(summary.objects_deleted, locale),
            })}
          </p>
          {summary.drain_timed_out && (
            // ds:ignore a warning in --warningText, not a refusal
            <p className="settings-danger-warning" role="alert">
              {t("settings.resetDataDrainWarning")}
            </p>
          )}
        </PanelBody>
      )}
      <ConfirmModal
        open={open}
        onClose={() => {
          setOpen(false);
          setTyped("");
          reset.reset();
        }}
        title={t("settings.resetDataConfirmTitle")}
        confirmLabel={t("settings.resetDataConfirmButton")}
        confirmVariant="danger"
        // The typed confirmation gates whether the reset may start. The input
        // stays editable while it runs. Clearing it mid-write must not re-arm
        // the gate on a control that is already going.
        confirmDisabled={!reset.isPending && typed.trim() === ""}
        onConfirm={() => reset.mutate()}
        pending={reset.isPending}
        error={reset.error ? problemMessageOf(reset.error, t) : null}
      >
        <p>{t("settings.resetDataConfirmBody")}</p>
        {workspaceName ? (
          <p>
            {t("settings.resetDataConfirmName")}{" "}
            {/* userSelect:all lets one click select the whole name to copy */}
            <code style={{ userSelect: "all" }}>{workspaceName}</code>
          </p>
        ) : null}
        <TextInput
          aria-label={t("settings.resetDataConfirmLabel")}
          value={typed}
          onChange={(event) => setTyped(event.target.value)}
        />
      </ConfirmModal>
    </Panel>
  );
}
