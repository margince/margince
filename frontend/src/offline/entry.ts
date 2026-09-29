// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Imported for the build to compile; the page carries it inline, never as a request.
import "./offline.css";
import { startTheme } from "../app/theme";
import { preferredLocale } from "../i18n/locale";
import { presentOfflinePage } from "./present";

startTheme();
presentOfflinePage(document, preferredLocale());
