// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The settings screen still reaches every card the catalog pages are built
// from. A card dropped while pages are split leaves every page drawing
// something, so no other gate sees the surface go.

import { join, sep } from "node:path";
import {
  moduleSpecifiers,
  resolveRelative,
} from "../../scripts/lib/source-tree";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  srcDir,
} from "../tree";

const settingsDir = join(srcDir, "features", "settings");
const settingsScreen = join(settingsDir, "settings.tsx");

// Every card the settings screen drew before its pages were split.
export const CARDS_THE_REGISTER_REACHED = [
  "AccountSettings",
  "AutonomySettingsCard",
  "VoiceDnaCard",
  "AiAccessTab",
  "InstallationSettingsCard",
  "FxRatesCard",
  "CompanyContextCard",
  "SignInMethodsCard",
  "OAuthAppsCard",
  "ExtensionAccessCard",
  "UsersAdminCard",
  "TeamsCard",
  // The cards of the connections and integrations tabs, one by one.
  "ConnectorsCard",
  "OwnerIdentitiesCard",
  "OvernightGrantCard",
  "LinkedInNetworkPanel",
  "ExtensionUnitsCard",
  "CaptureActivityTab",
  "ProviderCard",
  "FinanceSourceCard",
  "WebhooksCard",
  "TelegramConnectorsPanel",
  "OwnDomainsCard",
  "CaptureSettingsCard",
  "ConsumerMailDomainsCard",
  "BlockedDomainsCard",
  "CustomFieldsAdmin",
  "TagVocabularyCard",
  "PipelinesCard",
  "LeadOriginsCard",
  "LeadDisqualifyReasonsCard",
  "LeadHandlingCard",
  "ProductsAdmin",
  "OfferTemplatesAdmin",
  "KnowledgeCard",
  "AiRoutingCard",
  "AiProviderKeysCard",
  "AutomationsAdmin",
  "AiUsageCard",
  "AiCallsCard",
  "ConsentPurposesCard",
  "RetentionCard",
  "RestrictedRecordsCard",
  "PrivacyInboxCard",
  "AuditLogCard",
  "LicenseCard",
  "ImportCard",
  "EmbedReindexCard",
  "JobHealthCard",
  "ResetDataCard",
];

const inSettings = (path: string) => path.startsWith(settingsDir + sep);

/** The files settings.tsx reaches by imports inside its own folder. */
function reachedFromScreen(tree: CensusTree): CensusFile[] {
  const inTree = (path: string) => tree.at(path) !== undefined;
  const visited = new Map<string, CensusFile>();
  const pending = [settingsScreen];
  for (let path = pending.pop(); path !== undefined; path = pending.pop()) {
    const file = tree.at(path);
    if (!file || visited.has(path)) continue;
    visited.set(path, file);
    for (const specifier of file.source
      ? moduleSpecifiers(file.source, "all")
      : []) {
      const next = resolveRelative(path, specifier, inTree);
      if (next !== null && inSettings(next)) pending.push(next);
    }
  }
  return [...visited.values()];
}

/** Every component a JSX tag names in these files. */
export function renderedIn(files: readonly CensusFile[]): Set<string> {
  const text = files.map((file) => file.text).join("\n");
  return new Set(
    [...text.matchAll(/<([A-Z][A-Za-z]+)\b/g)].map((match) => match[1] ?? ""),
  );
}

export const rule: CensusRule = {
  name: "settingsdispatch",
  corpus: reachedFromScreen,
  // Two empty sets compare equal, so a scan that matched nothing would pass.
  floors: (corpus) =>
    renderedIn(corpus).size > 30
      ? []
      : ["30 rendered components or fewer were found from settings.tsx"],
  across: (corpus) => {
    const rendered = renderedIn(corpus);
    return CARDS_THE_REGISTER_REACHED.filter((card) => !rendered.has(card)).map(
      (card) => `${card} is no longer reached from settings.tsx`,
    );
  },
  advice:
    "A settings card is no longer drawn on any page. Mount it on the page it " +
    "belongs to, or drop it from CARDS_THE_REGISTER_REACHED if it was retired on purpose.",
};
