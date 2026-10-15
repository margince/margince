import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { FactList } from "../design-system/factlist";
import { FileChip } from "../design-system/filechip";
import { Markdown } from "../design-system/markdown";
import { Panel, PanelBody } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatBytes, formatDate, formatNumber } from "../format/format";
import {
  jurisdictionName,
  parsePeriod,
  ruleText,
} from "../format/jurisdiction";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import { problemMessageOf, throwProblem, unwrap } from "../lib/problem";
import { DSR_KIND_LABEL } from "./privacy.logic";
import {
  AUDIENCE_LABEL,
  DOCUMENT_KIND_LABEL,
  deadlineText,
  documentFilename,
  floorText,
} from "./settings.jurisdiction.text";

// Settings → Governance → Jurisdiction: which country's pack this installation
// runs under, what that selection puts in force, and the documents the pack
// ships. The server assembles every section from one reader
// (handlers_installation_jurisdiction.go); this screen only draws it.

type Jurisdiction = components["schemas"]["InstallationJurisdiction"];
type RetentionClass = components["schemas"]["JurisdictionRetentionClass"];

// The Select's value for "no pack". The empty string is the Select's "nothing
// chosen", so the explicit choice needs a value of its own; it is sent as "".
const NO_PACK = "none";

const QUERY_KEY = ["installation-jurisdiction"] as const;

export function JurisdictionCard() {
  const t = useT();
  const query = useQuery({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/installation/jurisdiction",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
  return (
    <QueryGate query={query} pendingLabel={t("jurisdiction.card.title")}>
      {(jurisdiction) => <JurisdictionReading jurisdiction={jurisdiction} />}
    </QueryGate>
  );
}

export function JurisdictionReading({
  jurisdiction,
}: Readonly<{ jurisdiction: Jurisdiction }>) {
  return (
    <>
      <SelectionPanel jurisdiction={jurisdiction} />
      <EffectsPanel jurisdiction={jurisdiction} />
      <DocumentsPanel jurisdiction={jurisdiction} />
    </>
  );
}

// Untitled: the page's own heading names it, and the panels after it are titled
// by what they hold.
function SelectionPanel({
  jurisdiction,
}: Readonly<{ jurisdiction: Jurisdiction }>) {
  const t = useT();
  const { locale } = useLocale();
  const canChange = useCanWrite("installation_settings", "update");
  const selected = jurisdiction.selected ?? null;
  return (
    <Panel>
      <SettingList bleed="settings">
        <SettingRow
          label={t("jurisdiction.selected.label")}
          description={
            selected === null
              ? t("jurisdiction.selected.noneDescription")
              : jurisdiction.unit?.description
          }
          value={
            selected === null
              ? t("jurisdiction.selected.none")
              : jurisdictionName(selected, locale)
          }
          control={
            canChange && <ChangeJurisdiction jurisdiction={jurisdiction} />
          }
        />
        {jurisdiction.unit?.unit_name && (
          <SettingRow
            label={t("jurisdiction.unit.label")}
            value={t("jurisdiction.unit.value", {
              name: jurisdiction.unit.unit_name,
              version: jurisdiction.unit.unit_version,
            })}
            control={null}
          />
        )}
        {/* A pack's documents get a panel of their own; their absence is one
            fact about the selection. */}
        {jurisdiction.documents.length === 0 && (
          <SettingRow
            label={t("jurisdiction.documents.title")}
            description={t(
              jurisdiction.selected
                ? "jurisdiction.documents.empty"
                : "jurisdiction.documents.emptyUnset",
            )}
            control={null}
          />
        )}
      </SettingList>
    </Panel>
  );
}

function ChangeJurisdiction({
  jurisdiction,
}: Readonly<{ jurisdiction: Jurisdiction }>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const [choice, setChoice] = useState("");
  const [open, setOpen] = useState(false);
  const current = jurisdiction.selected ?? "";
  const change = useMutation({
    mutationFn: async (country: string) =>
      unwrap(await api.PATCH("/installation/settings", { body: { country } })),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: QUERY_KEY });
      setOpen(false);
      setChoice("");
    },
  });
  const close = () => {
    setOpen(false);
    setChoice("");
    change.reset();
  };
  const target = choice === NO_PACK ? "" : choice;
  const options = [
    ...jurisdiction.available.map((unit) => ({
      value: unit.code,
      label: jurisdictionName(unit.code, locale),
    })),
    { value: NO_PACK, label: t("jurisdiction.option.none") },
  ].filter(
    (option) => (option.value === NO_PACK ? "" : option.value) !== current,
  );
  return (
    <>
      <Button aria-haspopup="dialog" onClick={() => setOpen(true)}>
        {t("jurisdiction.change.action")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={close}
        title={
          choice === ""
            ? t("jurisdiction.change.label")
            : t("jurisdiction.confirm.title", {
                name:
                  target === ""
                    ? t("jurisdiction.option.none")
                    : jurisdictionName(target, locale),
              })
        }
        confirmLabel={t("jurisdiction.confirm.confirm")}
        confirmDisabled={choice === ""}
        onConfirm={() => change.mutate(target)}
        pending={change.isPending}
        error={change.isError ? problemMessageOf(change.error, t) : null}
      >
        <Field
          label={t("jurisdiction.card.title")}
          hint={t("jurisdiction.change.description")}
        >
          {(control) => (
            <Select
              {...control}
              options={options}
              value={choice}
              placeholder={t("jurisdiction.change.placeholder")}
              onChange={setChoice}
            />
          )}
        </Field>
        {choice !== "" && (
          <ChangeConsequences jurisdiction={jurisdiction} target={target} />
        )}
      </ConfirmModal>
    </>
  );
}

/** What the confirm dialog lists: the floor before and after, and the rest. */
export function ChangeConsequences({
  jurisdiction,
  target,
}: Readonly<{ jurisdiction: Jurisdiction; target: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const before = jurisdiction.floor.known
    ? floorText(
        {
          name: jurisdiction.floor.class ?? "",
          keep: jurisdiction.floor.keep ?? "P0D",
          from_year_end: jurisdiction.floor.from_year_end,
        },
        locale,
        t,
      )
    : t("jurisdiction.floor.none");
  const afterClass = targetFloor(jurisdiction, target);
  const after = afterClass
    ? floorText(afterClass, locale, t)
    : t("jurisdiction.floor.none");
  const weakens = jurisdiction.floor.known && !afterClass;
  return (
    <>
      <p>{t("jurisdiction.confirm.floor", { from: before, to: after })}</p>
      {weakens && (
        <p>{t("jurisdiction.confirm.floorCarried", { from: before })}</p>
      )}
      <p>{t("jurisdiction.confirm.deadlines")}</p>
      <p>{t("jurisdiction.confirm.messaging")}</p>
    </>
  );
}

// The floor a target puts in force. "No pack" puts the strictest compiled-in
// one in force, which is the longest floor among the available packs.
function targetFloor(
  jurisdiction: Jurisdiction,
  target: string,
): RetentionClass | undefined {
  if (target !== "") {
    return jurisdiction.available.find((unit) => unit.code === target)
      ?.correspondence_floor;
  }
  return jurisdiction.available
    .map((unit) => unit.correspondence_floor)
    .filter((floor): floor is RetentionClass => floor !== undefined)
    .sort((a, b) => periodMonths(b.keep) - periodMonths(a.keep))[0];
}

function periodMonths(iso: string): number {
  const period = parsePeriod(iso);
  return period ? period.years * 12 + period.months + period.days / 31 : 0;
}

function EffectsPanel({
  jurisdiction,
}: Readonly<{ jurisdiction: Jurisdiction }>) {
  const t = useT();
  const { locale } = useLocale();
  const selected = jurisdiction.selected ?? null;
  const floor = jurisdiction.floor;
  const name = selected === null ? "" : jurisdictionName(selected, locale);
  return (
    <Panel title={t("jurisdiction.effects.title")}>
      <PanelBody>
        {!floor.known && (
          <Callout
            tone="warning"
            title={t("jurisdiction.effects.floorNone.title")}
          >
            {t("jurisdiction.effects.floorNone.body", { name })}
          </Callout>
        )}
        <FactList
          facts={[
            {
              key: "floor",
              term: t("jurisdiction.effects.floor"),
              value: floor.known
                ? floorText(
                    {
                      name: floor.class ?? "",
                      keep: floor.keep ?? "P0D",
                      from_year_end: floor.from_year_end,
                    },
                    locale,
                    t,
                  )
                : t("jurisdiction.floor.none"),
              note:
                selected === null
                  ? t("jurisdiction.effects.floorStrictest")
                  : t("jurisdiction.effects.floorPack", { name }),
            },
            ...(floor.carried
              ? [
                  {
                    key: "carried",
                    term: t("jurisdiction.effects.carried"),
                    value: t("jurisdiction.effects.carriedValue", {
                      floor: floorText(
                        {
                          name: floor.carried.class,
                          keep: floor.carried.keep,
                          from_year_end: floor.carried.from_year_end,
                        },
                        locale,
                        t,
                      ),
                      before: formatDate(
                        floor.carried.before,
                        locale,
                        viewerZone(),
                      ),
                      until: formatDate(
                        floor.carried.binds_until,
                        locale,
                        viewerZone(),
                      ),
                    }),
                  },
                ]
              : []),
            ...jurisdiction.deadlines.map((deadline) => ({
              key: `deadline-${deadline.kind}`,
              term: t("jurisdiction.effects.deadline", {
                kind: t(DSR_KIND_LABEL[deadline.kind]),
              }),
              value: deadlineText(deadline, locale, t),
              note: ruleText(deadline.rule, t),
            })),
            {
              key: "messaging",
              term: t("jurisdiction.effects.messaging"),
              value: jurisdiction.messaging
                ? t("jurisdiction.effects.messagingValue", {
                    version: formatNumber(
                      jurisdiction.messaging.version,
                      locale,
                    ),
                  })
                : t("jurisdiction.effects.messagingNone"),
              note: jurisdiction.messaging?.instruments
                .map((instrument) => instrument.name)
                .join(", "),
            },
            ...(jurisdiction.retention_classes ?? []).map((retentionClass) => ({
              key: `class-${retentionClass.name}`,
              term: t("jurisdiction.effects.class"),
              value: floorText(retentionClass, locale, t),
            })),
          ]}
        />
      </PanelBody>
    </Panel>
  );
}

function DocumentsPanel({
  jurisdiction,
}: Readonly<{ jurisdiction: Jurisdiction }>) {
  const t = useT();
  const { locale } = useLocale();
  const [reading, setReading] = useState<string | null>(null);
  const documents = jurisdiction.documents;
  if (documents.length === 0) {
    return null;
  }
  return (
    <Panel title={t("jurisdiction.documents.title")}>
      <PanelBody>
        <FactList
          facts={documents.map((document) => ({
            key: document.id,
            term: document.title,
            value: (
              <span className="form-actions">
                <FileChip
                  href={`/v1/installation/jurisdiction/documents/${encodeURIComponent(document.id)}`}
                  filename={documentFilename(document)}
                  size={formatBytes(document.size_bytes, locale)}
                />
                {document.media_type === "text/markdown" && (
                  <Button
                    variant="ghost"
                    onClick={() =>
                      setReading(reading === document.id ? null : document.id)
                    }
                  >
                    {t(
                      reading === document.id
                        ? "jurisdiction.documents.close"
                        : "jurisdiction.documents.read",
                    )}
                  </Button>
                )}
              </span>
            ),
            note: t("jurisdiction.documents.note", {
              audience: t(AUDIENCE_LABEL[document.audience]),
              kind: t(DOCUMENT_KIND_LABEL[document.kind]),
              language: document.language.toUpperCase(),
            }),
          }))}
        />
        {reading && <DocumentReader id={reading} />}
      </PanelBody>
    </Panel>
  );
}

// The in-page reader for a Markdown document, fetched when opened.
function DocumentReader({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const query = useQuery({
    queryKey: [...QUERY_KEY, "document", id],
    queryFn: async () => {
      const response = await fetch(
        `/v1/installation/jurisdiction/documents/${encodeURIComponent(id)}`,
        {
          credentials: "same-origin",
        },
      );
      if (!response.ok) {
        throw new Error(t("jurisdiction.documents.readFailed"));
      }
      return response.text();
    },
  });
  return (
    <QueryGate query={query} pendingLabel={t("jurisdiction.documents.title")}>
      {(source) => <Markdown source={source} />}
    </QueryGate>
  );
}
