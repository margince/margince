import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { Callout } from "../design-system/callout";
import { NumberSettingRow } from "../design-system/numbersetting";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

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

function useUpdateCaptureSettings() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    // A sparse patch rather than one boolean: the card now carries two
    // settings, and a mutation that could only send one would have to grow a
    // second copy of itself the moment a third arrives.
    mutationFn: async (patch: CaptureSettingsPatch) => {
      const { data, error } = await api.PATCH("/capture/settings", {
        body: patch,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
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

export function CaptureSettingsCard() {
  const t = useT();
  const canManage = useCanWrite("capture_settings", "update");
  const query = useCaptureSettings();
  const update = useUpdateCaptureSettings();

  // Panel rather than Card, and the gap to the card below comes from the
  // page's own stack (`.settings-stack`) rather than from a margin this card
  // carries — a surface that spaces itself is a surface that spaces itself
  // wrong the first time it is used anywhere else.
  //
  // Panel's header holds the title alone, so the card's one line of
  // description leads the body instead of riding in the header.
  return (
    <Panel title={t("captureSettings.title")}>
      <PanelBody>
        <PanelIntro>{t("captureSettings.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("captureSettings.title")}>
        {(settings) => (
          <SettingList bleed="settings">
            {/* The row draws the naming — what the setting is, and what it
                does — so the switch carries the same words hidden: it owns
                its own accessible name by design, and pointing it at the
                row's label as well would name it twice. */}
            <SettingRow
              label={t("captureSettings.autoEnrich.label")}
              description={t("captureSettings.autoEnrich.help")}
              control={
                <Switch
                  testId="capture-auto-enrich-toggle"
                  label={t("captureSettings.autoEnrich.label")}
                  labelHidden
                  // Two reasons, and only one of them is worth words: a
                  // caller who may never change this needs to know why,
                  // where a write already in flight explains itself by
                  // finishing.
                  reason={
                    canManage ? undefined : t("captureSettings.adminOnly")
                  }
                  checked={settings.auto_enrich}
                  disabled={!canManage || update.isPending}
                  onChange={(next) => update.mutate({ auto_enrich: next })}
                />
              }
            />
            {/* The workspace DEFAULT, and the description says so: a mailbox
                that set its own switch keeps it, so this row is not the whole
                answer for every connection and must not read as though it
                were. */}
            <SettingRow
              label={t("captureSettings.signatureEnrich.label")}
              description={t("captureSettings.signatureEnrich.help")}
              control={
                <Switch
                  testId="capture-signature-enrich-toggle"
                  label={t("captureSettings.signatureEnrich.label")}
                  labelHidden
                  reason={
                    canManage ? undefined : t("captureSettings.adminOnly")
                  }
                  checked={settings.signature_enrich}
                  disabled={!canManage || update.isPending}
                  onChange={(next) => update.mutate({ signature_enrich: next })}
                />
              }
            />
          </SettingList>
        )}
      </QueryGate>
      {update.isError && (
        <PanelBody>
          <Callout
            tone="danger"
            kind="outcome"
            title={t("captureSettings.updateFailed")}
          >
            {problemMessageOf(update.error, t)}
          </Callout>
        </PanelBody>
      )}
    </Panel>
  );
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

// How much the website reader may spend: the daily ceiling on reads nobody
// asked for, and what any one read may fetch. Same gate and same save as the
// switches above, because it is the same settings object.
export function WebsiteReadingCard() {
  const t = useT();
  const canManage = useCanWrite("capture_settings", "update");
  const query = useCaptureSettings();
  const update = useUpdateCaptureSettings();
  const locked = !canManage || update.isPending;
  return (
    <Panel title={t("captureReading.title")}>
      <PanelBody>
        <PanelIntro>{t("captureReading.sub")}</PanelIntro>
        {!canManage && (
          <PanelIntro>{t("captureSettings.adminOnly")}</PanelIntro>
        )}
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("captureReading.title")}>
        {(settings) => (
          <SettingList bleed="settings">
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
                onCommit={(next) => update.mutate({ [row.property]: next })}
              />
            ))}
          </SettingList>
        )}
      </QueryGate>
      {update.isError && (
        <PanelBody>
          <Callout
            tone="danger"
            kind="outcome"
            title={t("captureSettings.updateFailed")}
          >
            {problemMessageOf(update.error, t)}
          </Callout>
        </PanelBody>
      )}
    </Panel>
  );
}

// How often a connected mailbox is checked for new mail. Its own card rather
// than a row under enrichment: it paces every mailbox's capture, not what is
// done with a message once it arrives. Mirrored as READ_LIMITS is.
const MAIL_SYNC_BOUNDS = {
  mail_sync_interval_seconds: { min: 30, max: 3_600 },
} as const;

export function MailSyncCard() {
  const t = useT();
  const canManage = useCanWrite("capture_settings", "update");
  const query = useCaptureSettings();
  const update = useUpdateCaptureSettings();
  return (
    <Panel title={t("captureMailSync.title")}>
      <PanelBody>
        <PanelIntro>{t("captureMailSync.sub")}</PanelIntro>
        {!canManage && (
          <PanelIntro>{t("captureSettings.adminOnly")}</PanelIntro>
        )}
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("captureMailSync.title")}>
        {(settings) => (
          <SettingList bleed="settings">
            <NumberSettingRow
              label={t("captureMailSync.interval.label")}
              description={t("captureMailSync.interval.help")}
              testId="capture-mail-sync"
              value={settings.mail_sync_interval_seconds}
              {...MAIL_SYNC_BOUNDS.mail_sync_interval_seconds}
              refusal={t("captureMailSync.interval.refusal")}
              disabled={!canManage || update.isPending}
              onCommit={(next) =>
                update.mutate({ mail_sync_interval_seconds: next })
              }
            />
          </SettingList>
        )}
      </QueryGate>
      {update.isError && (
        <PanelBody>
          <Callout
            tone="danger"
            kind="outcome"
            title={t("captureSettings.updateFailed")}
          >
            {problemMessageOf(update.error, t)}
          </Callout>
        </PanelBody>
      )}
    </Panel>
  );
}
