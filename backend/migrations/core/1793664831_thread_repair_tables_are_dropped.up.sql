SET lock_timeout = '3s';
DROP FUNCTION activity_thread_available(text);
DROP TABLE capture_thread_invalidation, capture_thread_repair_node, capture_thread_topic, capture_thread_reply,
 capture_thread_barrier, capture_thread_repair, capture_thread_pending;
-- A basis the withdrawn repair copied loses the column that names its origin, so it
-- stops counting as consent here rather than outliving that proof.
UPDATE communication_basis SET revoked_at = statement_timestamp()
 WHERE thread_derived_from IS NOT NULL AND revoked_at IS NULL;
DROP TRIGGER communication_basis_source_erased ON communication_basis;
DROP FUNCTION communication_basis_source_erased();
ALTER TABLE capture_import DROP COLUMN thread_input_hash;
ALTER TABLE communication_basis DROP COLUMN thread_derived_from, DROP COLUMN thread_source_activity_id;
ALTER TABLE activity_mail_reference DROP COLUMN reference_order;
DROP INDEX activity_thread_repair_page, communication_basis_thread_repair_page;
-- Each wider index already serves the lookup the old name did; a rename builds nothing.
ALTER INDEX activity_identity_repair_page RENAME TO activity_identity_activity_idx;
ALTER INDEX activity_mail_reference_inverse RENAME TO activity_mail_reference_referenced_idx;
ALTER INDEX comms_outbound_thread_repair_page RENAME TO comms_outbound_workspace_activity_ix;
