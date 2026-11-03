SET lock_timeout = '3s';
DROP FUNCTION activity_thread_available(text);
DROP TABLE capture_thread_invalidation, capture_thread_repair_node, capture_thread_topic, capture_thread_reply,
 capture_thread_barrier, capture_thread_repair, capture_thread_pending;
DROP TRIGGER communication_basis_source_erased ON communication_basis;
DROP FUNCTION communication_basis_source_erased();
ALTER TABLE capture_import DROP COLUMN thread_input_hash;
ALTER TABLE communication_basis DROP COLUMN thread_derived_from, DROP COLUMN thread_source_activity_id;
ALTER TABLE activity_mail_reference DROP COLUMN reference_order;
DROP INDEX activity_thread_repair_page, activity_identity_repair_page, communication_basis_thread_repair_page,
 comms_outbound_thread_repair_page, activity_mail_reference_inverse;
CREATE INDEX activity_identity_activity_idx ON activity_identity (activity_id);
CREATE INDEX activity_mail_reference_referenced_idx ON activity_mail_reference (referenced_id);
CREATE INDEX comms_outbound_workspace_activity_ix ON comms_outbound (activity_id);
