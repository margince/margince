// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { LucideIcon } from "lucide-react";
import {
  Activity,
  BadgeCheck,
  Blocks,
  BookOpen,
  Building2,
  Database,
  Gauge,
  KeyRound,
  Mail,
  Mic,
  Plug,
  ShieldCheck,
  Sparkles,
  UserRound,
  UsersRound,
  Webhook,
  Wrench,
} from "lucide-react";
import type { SettingsPageId } from "./settingscatalog";

/**
 * A lucide glyph per catalog page.
 *
 * Here rather than in the catalog because the catalog is React-free by
 * construction — it is imported by a node test and a plain script, and one
 * lucide import would end that. A page missing from this map is a TypeScript
 * error, so the table cannot fall behind the catalog silently.
 */
export const PAGE_ICONS: Readonly<Record<SettingsPageId, LucideIcon>> = {
  account: UserRound,
  voice: Mic,
  agents: KeyRound,
  connections: Plug,
  "capture-activity": Activity,
  company: Building2,
  authentication: ShieldCheck,
  members: UsersRound,
  teams: UsersRound,
  seats: BadgeCheck,
  pipelines: Database,
  stageautomation: Gauge,
  leads: Database,
  acquisition: Database,
  fields: Database,
  tags: Database,
  products: Database,
  capture: Mail,
  integrations: Webhook,
  knowledge: BookOpen,
  import: Database,
  models: Sparkles,
  automations: Sparkles,
  usage: Sparkles,
  "model-calls": Sparkles,
  privacy: ShieldCheck,
  audit: ShieldCheck,
  "system-health": Wrench,
  extensions: Blocks,
  reset: Wrench,
};
