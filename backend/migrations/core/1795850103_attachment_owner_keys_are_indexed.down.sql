-- pgmigrate:no-transaction
-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_owner_contact;
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_owner_company;
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_owner_deal;
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_owner_lead;
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_owner_project;
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_owner_activity;
