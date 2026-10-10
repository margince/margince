import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { useCanWrite } from "../app/capability";
import { Badge, OverflowMenu } from "../design-system/atoms";
import type { ListColumn } from "../design-system/listtable";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { type Translator, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { ArchiveAction } from "./archive";
import { unwrap, useMe } from "./common";
import { CreateAction, type CreateField } from "./create";
import { EditAction } from "./edit";
import {
  type ListPage,
  type ListQuery,
  ListTable,
  listFetchLimit,
  useListQuery,
} from "./listquery";
import "./listsection.css";

type OfferTemplate = components["schemas"]["OfferTemplate"];

async function fetchTemplatesPage(
  query: ListQuery,
  cursor: string | null,
): Promise<ListPage<OfferTemplate>> {
  const data = unwrap(
    await api.GET("/offer-templates", {
      params: {
        query: {
          sort: query.sort || undefined,
          include_archived: query.includeArchived || undefined,
          cursor: cursor || undefined,
          limit: listFetchLimit(query.perPage),
          ...query.filters,
        },
      },
    }),
  );
  return {
    data: data.data,
    page: {
      next_cursor: data.page.next_cursor ?? null,
      has_more: data.page.has_more,
    },
  };
}

const LOCALE_OPTIONS: { value: string; label: MessageKey }[] = [
  { value: "de-DE", label: "template.localeDE" },
  { value: "en-US", label: "template.localeEN" },
];

function languageOf(locale: string, t: Translator): string {
  const known = LOCALE_OPTIONS.find((option) => option.value === locale);
  return known ? t(known.label) : locale;
}

// Built per render: option labels are shown as given, so they arrive translated.
function templateFields(t: Translator): CreateField[] {
  return [
    { key: "name", label: "template.name", required: true },
    {
      key: "locale",
      label: "template.locale",
      type: "select",
      required: true,
      options: LOCALE_OPTIONS.map(({ value, label }) => ({
        value,
        label: t(label),
      })),
    },
    {
      key: "is_default",
      label: "template.isDefault",
      type: "select",
      required: true,
      options: [
        { value: "false", label: t("field.no") },
        { value: "true", label: t("field.yes") },
      ],
    },
    // The offer PDF prints these two layout keys by these names;
    // backend/gates/offertemplatelayout_test.go holds the spellings together.
    { key: "header", label: "template.header" },
    { key: "footer", label: "template.footer" },
  ];
}

/**
 * The offer shells an offer is built from, as a section of Settings → Data model.
 *
 * Not a screen of its own, for the same reason as {@link ProductsAdmin}: it was
 * reached by a card that existed only to send you here. It renders no `.wrap` —
 * the settings page owns the reading column — and names itself in a Panel, the
 * same shape products takes, since the shell's page title now belongs to the
 * whole data-model page and the surfaces on it have to read as one kind of thing.
 */
export function OfferTemplatesAdmin() {
  const t = useT();
  // One grant per affordance, named for the request it issues: New POSTs, Edit
  // PUTs, Archive DELETEs. `useCanWrite` rather than `useCan` because all three
  // mutate, and the licensing seat is clamped on the HTTP method before RBAC is
  // reached — a read seat holding offer_template:update would otherwise open an
  // editor whose every save is refused.
  const me = useMe();
  const canCreate = useCanWrite("offer_template", "create");
  const canUpdate = useCanWrite("offer_template", "update");
  const canArchive = useCanWrite("offer_template", "delete");
  // Scoped for the same reason the products table beside it is: the two share
  // the settings Data-model tab, and so shared one parameter space.
  const list = useListQuery<OfferTemplate>({
    key: "offer-templates",
    fetchPage: fetchTemplatesPage,
    initialSort: "name",
    paramScope: "templates",
  });
  const fields = templateFields(t);

  const createTemplate = async (values: Record<string, string>) => {
    return unwrap(
      await api.POST("/offer-templates", {
        body: {
          name: values.name.trim(),
          locale: values.locale || "de-DE",
          is_default: values.is_default === "true",
          layout: {
            header: values.header || undefined,
            footer: values.footer || undefined,
          },
        },
      }),
    );
  };

  const updateTemplate =
    (tpl: OfferTemplate) => async (values: Record<string, unknown>) => {
      // PUT full-replace (unlike product's merge-PATCH): every writable
      // field is supplied on every call — an omitted one would reset it.
      return unwrap(
        await api.PUT("/offer-templates/{id}", {
          params: { path: { id: tpl.id }, ...ifMatch(tpl.version) },
          body: {
            name: String(values.name).trim(),
            locale: String(values.locale),
            is_default: values.is_default === "true",
            layout: {
              header: (values.header as string) || undefined,
              footer: (values.footer as string) || undefined,
            },
          },
        }),
      );
    };

  // The per-row affordances, in a column that exists only while at least one of
  // them does: an emptied actions column would still take width, carry a header
  // and appear in the column picker, which reads as a table that lost its
  // buttons rather than as a permission.
  const rowMenu: ListColumn<OfferTemplate> = {
    key: "actions",
    header: t("table.actions"),
    verbs: "menu",
    cell: (tpl: OfferTemplate) => (
      <span className="cell-actions">
        <OverflowMenu label={t("template.rowActions", { name: tpl.name })}>
          {canUpdate && (
            <EditAction
              labelled
              label={t("template.edit")}
              savedMessage={(saved) =>
                t("record.saveDone", { name: saved.name })
              }
              invalidate="offer-templates"
              recordKey="offer-template"
              record={{
                ...tpl,
                is_default: String(tpl.is_default),
                // The contract types layout as an index signature, so its
                // members read straight off it. A cast would restate the schema.
                header: tpl.layout.header ?? "",
                footer: tpl.layout.footer ?? "",
              }}
              update={updateTemplate(tpl)}
              fields={fields}
            />
          )}
          {canArchive && (
            <ArchiveAction
              label={t("template.archive")}
              confirmText={t("template.archiveConfirm")}
              archivedMessage={t("record.archiveDone", { name: tpl.name })}
              invalidate="offer-templates"
              recordKey="offer-template"
              onArchived={() => list.refetch()}
              archive={async () => {
                const data = unwrap(
                  await api.DELETE("/offer-templates/{id}", {
                    params: { path: { id: tpl.id } },
                  }),
                );
                return data ?? tpl;
              }}
            />
          )}
        </OverflowMenu>
      </span>
    ),
  };

  return (
    <Panel className="listsection" title={t("template.title")}>
      <PanelBody className="listsection-intro">
        <PanelIntro>{t("template.settingsSub")}</PanelIntro>
        {/* Stated once for the whole section (design-system README, "Absent,
            disabled, or withheld"): a readable list whose editors are all withheld
            has to say so, or their absence reads as a claim about the list rather
            than about authority. A reader holding one write verb needs no notice —
            the affordance they keep says what they may do. */}
        {/* me.isSuccess first: every capability hook fails CLOSED while the
            probe is in flight, so branching on the grants alone flashes a
            read-only notice at the admin who holds all three. Gate on the
            probe, not on its absence. */}
        {me.isSuccess && !canCreate && !canUpdate && !canArchive && (
          <PanelIntro>{t("template.readOnly")}</PanelIntro>
        )}
      </PanelBody>
      <ListTable
        state={list}
        unit="unit.offerTemplates"
        searchable={false}
        action={
          canCreate ? (
            <CreateAction
              label={t("template.new")}
              invalidate="offer-templates"
              // `stay` because a fresh template has no page of its own to
              // open: the list the row joins is already on screen, and this
              // panel is one of the settings surfaces the screen names.
              screen="settings"
              stay
              create={createTemplate}
              fields={fields}
            />
          ) : undefined
        }
        columns={[
          {
            key: "name",
            header: t("template.name"),
            sort: "name",
            cell: (tpl: OfferTemplate) => tpl.name,
            fixed: true,
          },
          {
            key: "locale",
            header: t("template.locale"),
            sort: "locale",
            cell: (tpl: OfferTemplate) => languageOf(tpl.locale, t),
          },
          {
            key: "is_default",
            header: t("template.default"),
            sort: "is_default",
            cell: (tpl: OfferTemplate) =>
              tpl.is_default ? <Badge>{t("template.default")}</Badge> : null,
          },
          ...(canUpdate || canArchive ? [rowMenu] : []),
        ]}
        rowKey={(tpl) => tpl.id}
        chips={[
          {
            key: "locale",
            label: "template.localeFilter",
            allLabel: "template.localeFilterAll",
            options: LOCALE_OPTIONS,
          },
        ]}
      />
    </Panel>
  );
}
