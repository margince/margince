import { ArrowRight } from "lucide-react";
import {
  Badge,
  Button,
  Checkbox,
  Radio,
  TextInput,
} from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { WriteRefused } from "../../design-system/querystates";
import { FieldDiff } from "../../design-system/trust";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { coldFieldLabel } from "../settings/coldfieldlabels";
import type {
  CompanyInput,
  Comparison,
  Resolution,
  SiteRead,
} from "./company-context";
import { ReadWarnings } from "./company-context.notices";
import { trimCompanyInput } from "./company-context.profile";

export function RefreshReview(
  props: Readonly<{
    read: SiteRead;
    selected: Set<string>;
    resolutions: Record<string, Resolution>;
    onToggle: (key: string) => void;
    onResolve: (resolution: Resolution) => void;
    onConfirm: () => void;
    /** The same answer the save asks for: applying a read writes the profile.
     *  Carried rather than re-derived, so a grant revoked while a reviewer sits
     *  on this screen takes the apply with it. The /me snapshot refreshes on
     *  window focus, and the review outlives that. */
    canApply: boolean;
    confirming: boolean;
    error?: string;
  }>,
) {
  const t = useT();
  const { locale } = useLocale();
  const ready =
    props.read.status === "ready" || props.read.status === "partial";
  const conflicts = props.read.comparisons.filter(
    (item) => item.classification === "human_conflict",
  );
  const unresolved = conflicts.some((item) => {
    const resolution = props.resolutions[item.key];
    return (
      !resolution ||
      (resolution.action === "use_value" && !(resolution.value ?? "").trim())
    );
  });
  const coverage =
    props.read.pages.length === 0
      ? 0
      : Math.round(
          (props.read.pages.filter((page) => page.status === "fetched").length /
            props.read.pages.length) *
            100,
        );
  return (
    // The review as a Panel: the state sentence is its title, since a bare h3
    // draws at body size. The comparisons are full-bleed rows. The coverage
    // figure sits in the footer band, because it belongs to the whole read
    // rather than to one row, and nobody acts on it.
    <Panel
      title={
        ready
          ? t("settings.companyRefreshReady")
          : t("settings.companyRefreshReading")
      }
      footer={
        <span className="company-context-coverage">
          <strong>{formatNumber(coverage, locale)}%</strong>{" "}
          {t("settings.companyCoverage")}
        </span>
      }
      actions={
        <>
          {props.error && (
            <WriteRefused
              titleKey="settings.companyApplyFailed"
              message={props.error}
            />
          )}
          {/* An unresolved conflict refuses the verb, so it is the verb's own
              `reason` rather than a second notice beside it. The sentence and
              the refusal then cannot disagree. */}
          {props.canApply && (
            <Button
              variant="primary"
              disabled={!ready || props.confirming}
              reason={unresolved ? t("settings.companyResolveAll") : undefined}
              onClick={props.onConfirm}
            >
              {t("settings.companyApplyRefresh")} <ArrowRight aria-hidden />
            </Button>
          )}
        </>
      }
    >
      {/* What this panel is, under the title that says where the read has got
          to. Beside the title it would squeeze a sentence into a column on a
          phone, for the same reason the profile panel's eyebrow sits here. */}
      <PanelBody className="form-stack">
        <Eyebrow>{t("settings.companyRefreshReview")}</Eyebrow>
        <ReadWarnings warnings={props.read.warnings} />
      </PanelBody>
      {props.read.comparisons.map((item) => (
        <ComparisonRow
          key={`${item.value_kind}:${item.key}`}
          item={item}
          selected={props.selected.has(item.key)}
          resolution={props.resolutions[item.key]}
          onToggle={() => props.onToggle(item.key)}
          onResolve={props.onResolve}
        />
      ))}
    </Panel>
  );
}

function ComparisonRow(
  props: Readonly<{
    item: Comparison;
    selected: boolean;
    resolution?: Resolution;
    onToggle: () => void;
    onResolve: (resolution: Resolution) => void;
  }>,
) {
  const t = useT();
  const { item } = props;
  const conflict = item.classification === "human_conflict";
  // The field this card is about, named once: it heads the card and names the
  // checkbox. A screen reader cannot tell apart boxes that all announce the
  // same words, and the change picked here is written to the record.
  const fieldLabel = coldFieldLabel(item.key.split("/").at(-2) ?? item.key, t);
  const selectable = !conflict && item.classification !== "unchanged";
  return (
    <PanelRow
      className={`company-context-comparison is-${item.classification}`}
    >
      <div className="company-context-comparison-title">
        {/* On a selectable row the field name is the tick's other half, which
            makes the words clickable. The aria-label spells the whole
            instruction and contains those words. So the visible name and the
            announced one agree (WCAG 2.5.3). A row with nothing to choose keeps
            the name as plain text rather than a control that does nothing. */}
        {selectable ? (
          <Checkbox
            checked={props.selected}
            onChange={props.onToggle}
            aria-label={t("settings.companySelectChange", {
              field: fieldLabel,
            })}
            label={<strong>{fieldLabel}</strong>}
          />
        ) : (
          <strong>{fieldLabel}</strong>
        )}
        <Badge>
          {t(`settings.companyClass.${item.classification}` as MessageKey)}
        </Badge>
      </div>
      {/* The design system's own old→new diff. A null current value reads as
          the "created" marker rather than as a blank box claiming we held an
          empty string. */}
      <FieldDiff oldValue={item.current_value} newValue={item.proposed_value} />
      {conflict && (
        <div className="company-context-resolutions">
          {(["keep_current", "accept_proposal"] as const).map((action) => (
            <Radio
              key={action}
              name={`resolution-${item.key}`}
              checked={props.resolution?.action === action}
              onChange={() => props.onResolve({ key: item.key, action })}
              label={t(`settings.companyResolution.${action}` as MessageKey)}
            />
          ))}
          <Radio
            name={`resolution-${item.key}`}
            checked={props.resolution?.action === "use_value"}
            onChange={() =>
              props.onResolve({
                key: item.key,
                action: "use_value",
                value: item.current_value ?? "",
              })
            }
            label={t("settings.companyResolution.use_value")}
          />
          {props.resolution?.action === "use_value" && (
            /* Named, because this field decides what gets written to the
               company record, and it has no label or placeholder of its own.
               The radio above reveals it, so it takes that radio's words plus
               the field this conflict is about. "Keep this value" alone would
               be one of several identical names on a page with several
               conflicts. */
            <TextInput
              aria-label={t("settings.companyResolution.useValueFor", {
                field: fieldLabel,
              })}
              value={props.resolution.value ?? ""}
              onChange={(event) =>
                props.onResolve({
                  key: item.key,
                  action: "use_value",
                  value: event.target.value,
                })
              }
            />
          )}
        </div>
      )}
    </PanelRow>
  );
}

export function refreshConfirmation(
  current: CompanyInput,
  read: SiteRead,
  selected: Set<string>,
  resolutions: Record<string, Resolution>,
) {
  const profile = { ...current };
  for (const comparison of read.comparisons) {
    if (comparison.value_kind !== "profile_field") {
      continue;
    }
    if (
      selected.has(comparison.key) &&
      comparison.classification !== "human_conflict"
    ) {
      profile[comparison.key as keyof CompanyInput] = comparison.proposed_value;
    }
  }
  const factKeys = read.facts
    .filter((fact) => selected.has(fact.value_key))
    .map((fact) => fact.value_key);
  return {
    draft_version: read.draft_version,
    proposal_hash: read.proposal_hash,
    profile: trimCompanyInput(profile),
    selected_fact_keys: factKeys,
    resolutions: Object.values(resolutions),
  };
}
