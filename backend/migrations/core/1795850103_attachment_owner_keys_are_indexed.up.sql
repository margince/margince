-- pgmigrate:no-transaction
-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_owner_contact;
CREATE INDEX CONCURRENTLY idx_attachment_owner_contact ON attachment (owner_contact_id) WHERE owner_contact_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_owner_company;
CREATE INDEX CONCURRENTLY idx_attachment_owner_company ON attachment (owner_company_id) WHERE owner_company_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_owner_deal;
CREATE INDEX CONCURRENTLY idx_attachment_owner_deal ON attachment (owner_deal_id) WHERE owner_deal_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_owner_lead;
CREATE INDEX CONCURRENTLY idx_attachment_owner_lead ON attachment (owner_lead_id) WHERE owner_lead_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_owner_project;
CREATE INDEX CONCURRENTLY idx_attachment_owner_project ON attachment (owner_project_id) WHERE owner_project_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_owner_activity;
CREATE INDEX CONCURRENTLY idx_attachment_owner_activity ON attachment (owner_activity_id) WHERE owner_activity_id IS NOT NULL;
