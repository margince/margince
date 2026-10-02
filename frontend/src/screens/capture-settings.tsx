import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { Callout } from "../design-system/callout";
import { NumberSetting } from "../design-system/numbersetting";
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
      {/* `form-stack` still earns its place: the failure Callout below the list
          is a non-row child, and without the body's gap it would butt against
          the last row's hairline. `.panel-intro`'s own interval is
          already corrected for a `.form-stack` body, so the description lands
          on the same 16px it does in a plain one. */}
      <PanelBody className="form-stack">
        <PanelIntro>{t("captureSettings.sub")}</PanelIntro>
        <QueryGate query={query} pendingLabel={t("captureSettings.title")}>
          {(settings) => (
            <SettingList>
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
                    onChange={(next) =>
                      update.mutate({ signature_enrich: next })
                    }
                  />
                }
              />
            </SettingList>
          )}
        </QueryGate>
        {update.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("captureSettings.updateFailed")}
          >
            {problemMessageOf(update.error, t)}
          </Callout>
        )}
      </PanelBody>
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
      <PanelBody className="form-stack">
        <PanelIntro>{t("captureReading.sub")}</PanelIntro>
        {!canManage && (
          <PanelIntro>{t("captureSettings.adminOnly")}</PanelIntro>
        )}
        <QueryGate query={query} pendingLabel={t("captureReading.title")}>
          {(settings) => (
            <SettingList>
              <SettingRow
                label={t("captureReading.dailyCap.label")}
                description={t("captureReading.dailyCap.help")}
                control={(control) => (
                  <NumberSetting
                    control={control}
                    testId="capture-daily-cap"
                    value={settings.auto_enrich_daily_cap}
                    {...READ_LIMITS.auto_enrich_daily_cap}
                    refusal={t("captureReading.dailyCap.refusal")}
                    disabled={locked}
                    onCommit={(next) =>
                      update.mutate({ auto_enrich_daily_cap: next })
                    }
                  />
                )}
              />
              <SettingRow
                label={t("captureReading.maxPages.label")}
                description={t("captureReading.maxPages.help")}
                control={(control) => (
                  <NumberSetting
                    control={control}
                    testId="capture-read-max-pages"
                    value={settings.site_read.max_pages}
                    {...READ_LIMITS.site_read_max_pages}
                    refusal={t("captureReading.maxPages.refusal")}
                    disabled={locked}
                    onCommit={(next) =>
                      update.mutate({ site_read_max_pages: next })
                    }
                  />
                )}
              />
              <SettingRow
                label={t("captureReading.maxMiB.label")}
                description={t("captureReading.maxMiB.help")}
                control={(control) => (
                  <NumberSetting
                    control={control}
                    testId="capture-read-max-mib"
                    value={settings.site_read.max_mib}
                    {...READ_LIMITS.site_read_max_mib}
                    refusal={t("captureReading.maxMiB.refusal")}
                    disabled={locked}
                    onCommit={(next) =>
                      update.mutate({ site_read_max_mib: next })
                    }
                  />
                )}
              />
              <SettingRow
                label={t("captureReading.wall.label")}
                description={t("captureReading.wall.help")}
                control={(control) => (
                  <NumberSetting
                    control={control}
                    testId="capture-read-wall"
                    value={settings.site_read.wall_seconds}
                    {...READ_LIMITS.site_read_wall_seconds}
                    refusal={t("captureReading.wall.refusal")}
                    disabled={locked}
                    onCommit={(next) =>
                      update.mutate({ site_read_wall_seconds: next })
                    }
                  />
                )}
              />
            </SettingList>
          )}
        </QueryGate>
        {update.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("captureSettings.updateFailed")}
          >
            {problemMessageOf(update.error, t)}
          </Callout>
        )}
      </PanelBody>
    </Panel>
  );
}
