// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Imported for the build to compile; the page carries it inline, never as a request.
import "./offline.css";
import { readStored, STORAGE_KEYS } from "../app/storage";
import { presentOfflinePage } from "./present";

presentOfflinePage(
  document,
  readStored(STORAGE_KEYS.locale),
  navigator.languages,
);
