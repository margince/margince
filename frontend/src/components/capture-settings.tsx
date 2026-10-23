import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { describedByIds } from "../design-system/describedby";
import { NumberSettingRow } from "../design-system/numbersetting";
import { Panel, PanelGroupHead } from "../design-system/panel";
import { PanelNotices } from "../design-system/panelnotices";
import { QueryGate } from "../design-system/querystates";
import { useSavePage } from "../design-system/savepage";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useToast } from "../design-system/toast";
import { lockReasonOr } from "../format/settinglocks";
import { useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import { throwProblem, unwrap } from "../lib/problem";

// The company capture-settings card (CAP-WIRE-7, ADR-0072): the
// captured-company auto-enrich toggle. Every role reads it; only admin/ops
// may change it, so the toggle is refused (never hidden) for other roles — a
// rep still sees whether auto-enrich is on. Mirrors the WebhooksCard gating.

// Exported because the connectors card reads it too: a mailbox row has to say
// whether its switch is showing that mailbox's own answer or the company's,
// and two queries against one path are two answers that can disagree on screen.
export function useCaptureSettings() {
  return useQuery({
    queryKey: ["capture-settings"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/capture/settings");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

type CaptureSettingsPatch =
  components["schemas"]["UpdateCaptureSettingsRequest"];

export function useUpdateCaptureSettings() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    // A sparse patch rather than one boolean: the card now carries two
    // settings, and a mutation that could only send one would have to grow a
    // second copy of itself the moment a third arrives.
    mutationFn: async (patch: CaptureSettingsPatch) => {
      return unwrap(
        await api.PATCH("/capture/settings", {
          body: patch,
        }),
      );
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["capture-settings"], data);
      // Two cards save into this one record. Answers can land out of order,
      // so the record is read again rather than trusting whichever came last.
      void queryClient.invalidateQueries({ queryKey: ["capture-settings"] });
      toast.show(t("settings.saved"));
    },
  });
}

// The limits the API refuses past, mirrored so a value out of range is
// refused in the box before the request. Keyed by the wire property, which is
// how backend/gates/settingbounds_test.go holds each to the contract.
const READ_LIMITS = {
  auto_enrich_daily_cap: { min: 1, max: 20_000 },
  site_read_max_pages: { min: 1, max: 200 },
  site_read_max_mib: { min: 1, max: 128 },
  site_read_wall_seconds: { min: 30, max: 600 },
} as const;

// One row per limit, in the order a reader decides them: how many reads a day,
// then how much each read may take.
const READ_ROWS: readonly Readonly<{
  property: keyof typeof READ_LIMITS;
  copy: "dailyCap" | "maxPages" | "maxMiB" | "wall";
  testId: string;
  value: (settings: components["schemas"]["CaptureSettings"]) => number;
}>[] = [
  {
    property: "auto_enrich_daily_cap",
    copy: "dailyCap",
    testId: "capture-daily-cap",
    value: (s) => s.auto_enrich_daily_cap,
  },
  {
    property: "site_read_max_pages",
    copy: "maxPages",
    testId: "capture-read-max-pages",
    value: (s) => s.site_read.max_pages,
  },
  {
    property: "site_read_max_mib",
    copy: "maxMiB",
    testId: "capture-read-max-mib",
    value: (s) => s.site_read.max_mib,
  },
  {
    property: "site_read_wall_seconds",
    copy: "wall",
    testId: "capture-read-wall",
    value: (s) => s.site_read.wall_seconds,
  },
];

// The enrichment switches and the website reader's limits: one settings
// object, so one card, one gate and one save.
export function CaptureSettingsCard() {
  const t = useT();
  const canManage = useCanWrite("capture_settings", "update");
  const query = useCaptureSettings();
  const update = useUpdateCaptureSettings();
  // In a page the bar shows a refused number, so the card would say it twice.
  const inPage = useSavePage() !== null;
  const refusedNumber = READ_ROWS.some(
    (row) => update.variables?.[row.property] !== undefined,
  );
  const locked = !canManage || update.isPending;
  const adminOnly = canManage ? undefined : t("captureSettings.adminOnly");
  const reasonFor = (
    settings: components["schemas"]["CaptureSettings"],
    property: string,
  ) => lockReasonOr(settings.locks, `capture.${property}`, t, adminOnly);
  const refusalId = useId();
  const refusedFor = (setting: keyof CaptureSettingsPatch) =>
    update.isError && update.variables?.[setting] !== undefined
      ? refusalId
      : undefined;

  return (
    <Panel title={t("captureSettings.title")}>
      <QueryGate query={query} pendingLabel={t("captureSettings.title")}>
        {(settings) => (
          <>
            <SettingList bleed="settings">
              {/* The switch carries its own hidden label; pointing it at the
                  row's label too would name it twice. */}
              <SettingRow
                label={t("captureSettings.autoEnrich.label")}
                description={t("captureSettings.autoEnrich.help")}
                control={(control) => (
                  <Switch
                    describedBy={describedByIds(
                      control["aria-describedby"],
                      refusedFor("auto_enrich"),
                    )}
                    testId="capture-auto-enrich-toggle"
                    label={t("captureSettings.autoEnrich.label")}
                    labelHidden
                    reason={reasonFor(settings, "auto_enrich")}
                    checked={settings.auto_enrich}
                    disabled={locked}
                    onChange={(next) => update.mutate({ auto_enrich: next })}
                  />
                )}
              />
              <SettingRow
                label={t("captureSettings.signatureEnrich.label")}
                description={t("captureSettings.signatureEnrich.help")}
                control={(control) => (
                  <Switch
                    describedBy={describedByIds(
                      control["aria-describedby"],
                      refusedFor("signature_enrich"),
                    )}
                    testId="capture-signature-enrich-toggle"
                    label={t("captureSettings.signatureEnrich.label")}
                    labelHidden
                    reason={reasonFor(settings, "signature_enrich")}
                    checked={settings.signature_enrich}
                    disabled={locked}
                    onChange={(next) =>
                      update.mutate({ signature_enrich: next })
                    }
                  />
                )}
              />
            </SettingList>
            <PanelGroupHead title={t("captureReading.title")} level="h3" />
            <SettingList bleed="settings" testId="capture-website-reads">
              {READ_ROWS.map((row) => (
                <NumberSettingRow
                  key={row.property}
                  label={t(`captureReading.${row.copy}.label`)}
                  description={t(`captureReading.${row.copy}.help`)}
                  testId={row.testId}
                  value={row.value(settings)}
                  {...READ_LIMITS[row.property]}
                  refusal={t(`captureReading.${row.copy}.refusal`)}
                  disabled={locked}
                  reason={reasonFor(settings, row.property)}
                  describedBy={inPage ? undefined : refusedFor(row.property)}
                  onCommit={(next) =>
                    inPage
                      ? update.mutateAsync({ [row.property]: next })
                      : update.mutate({ [row.property]: next })
                  }
                />
              ))}
            </SettingList>
          </>
        )}
      </QueryGate>
      <PanelNotices
        refused={
          update.isError && !(inPage && refusedNumber)
            ? {
                title: t("captureSettings.updateFailed"),
                error: update.error,
                id: refusalId,
              }
            : undefined
        }
      />
    </Panel>
  );
}

// How often a connected mailbox is checked for new mail, and how many held
// threads one privacy-review run looks at, mirrored as READ_LIMITS is. Both
// sit on the mail card, which draws them from here.
export const MAIL_SYNC_BOUNDS = {
  mail_sync_interval_seconds: { min: 30, max: 3_600 },
} as const;

export const MAIL_REVIEW_BOUNDS = {
  confidentiality_pass_cap: { min: 1, max: 200 },
} as const;
