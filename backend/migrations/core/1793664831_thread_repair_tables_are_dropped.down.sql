ALTER INDEX comms_outbound_workspace_activity_ix RENAME TO comms_outbound_thread_repair_page;
ALTER INDEX activity_mail_reference_referenced_idx RENAME TO activity_mail_reference_inverse;
ALTER INDEX activity_identity_activity_idx RENAME TO activity_identity_repair_page;
ALTER TABLE capture_import ADD COLUMN thread_input_hash text NOT NULL DEFAULT '';
ALTER TABLE communication_basis ADD COLUMN thread_derived_from uuid REFERENCES communication_basis(id) ON DELETE CASCADE;
ALTER TABLE communication_basis ADD COLUMN thread_source_activity_id uuid;
CREATE INDEX communication_basis_thread_derived_source ON communication_basis(coalesce(thread_source_activity_id,source_activity_id),id);
CREATE INDEX communication_basis_thread_origin ON communication_basis(thread_derived_from) WHERE thread_derived_from IS NOT NULL;
CREATE FUNCTION communication_basis_source_erased() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.source_activity_id IS NOT NULL AND NEW.source_activity_id IS NULL THEN
  NEW.revoked_at=coalesce(OLD.revoked_at,NEW.revoked_at,statement_timestamp());
  NEW.thread_source_activity_id=NULL;
  NEW.note=NULL;
 END IF;
 RETURN NEW;
END
$$;
CREATE TRIGGER communication_basis_source_erased BEFORE UPDATE OF source_activity_id ON communication_basis
FOR EACH ROW EXECUTE FUNCTION communication_basis_source_erased();
ALTER TABLE activity_mail_reference ADD COLUMN reference_order integer NOT NULL DEFAULT 0 CHECK(reference_order >= 0);
CREATE INDEX activity_thread_repair_page ON activity(thread_key,id) WHERE kind='email';
CREATE INDEX communication_basis_thread_repair_page ON communication_basis(thread_key,id) WHERE revoked_at IS NULL;
CREATE INDEX activity_mail_reference_ordered ON activity_mail_reference(activity_id,reference_order,referenced_id);

CREATE TABLE capture_thread_pending (source_key text PRIMARY KEY, blocked boolean NOT NULL DEFAULT false, queue_order bigint GENERATED ALWAYS AS IDENTITY);
CREATE INDEX capture_thread_pending_fifo ON capture_thread_pending(queue_order);
CREATE TABLE capture_thread_repair (
 id uuid PRIMARY KEY,
 created_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 was_blocked boolean NOT NULL DEFAULT false,
 state text NOT NULL DEFAULT 'ready' CHECK(state IN ('ready','backoff','quarantined')),
 failures integer NOT NULL DEFAULT 0 CHECK(failures BETWEEN 0 AND 3),
 retry_after timestamptz,
 phase text NOT NULL DEFAULT 'load' CHECK (phase IN ('load','discover','topics','labels','roots','targets','inherit','apply','repair','refresh','published')),
 after_activity uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 graph_stage text NOT NULL DEFAULT 'reset',
 graph_cursor uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 graph_cycle uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 graph_best_key text NOT NULL DEFAULT '',
 graph_best_topic text NOT NULL DEFAULT '',
 run_id uuid NOT NULL
);
CREATE UNIQUE INDEX capture_thread_one_active ON capture_thread_repair ((true)) WHERE phase <> 'published' AND state='ready';
CREATE INDEX capture_thread_retry_page ON capture_thread_repair(retry_after,id) WHERE state='backoff';
CREATE INDEX capture_thread_quarantine_page ON capture_thread_repair(id) WHERE state='quarantined';
CREATE TABLE capture_thread_barrier (
 repair_id uuid NOT NULL REFERENCES capture_thread_repair(id) ON DELETE RESTRICT,
 blocked_key text NOT NULL,
 after_activity uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 loaded boolean NOT NULL DEFAULT false,
 PRIMARY KEY (repair_id,blocked_key)
);
CREATE INDEX capture_thread_barrier_load_page ON capture_thread_barrier(repair_id,loaded,blocked_key);
CREATE INDEX capture_thread_barrier_key ON capture_thread_barrier(blocked_key,repair_id);
CREATE TABLE capture_thread_reply (activity_id uuid PRIMARY KEY REFERENCES activity(id) ON DELETE CASCADE, emitted boolean NOT NULL DEFAULT false);
CREATE TABLE capture_thread_topic (
 activity_id uuid PRIMARY KEY REFERENCES activity(id) ON DELETE CASCADE,
 own_topic text NOT NULL,
 recognized_keys text[] NOT NULL DEFAULT '{}',
 subject_hash text NOT NULL DEFAULT '',
 effective_topic text NOT NULL,
 root_key text NOT NULL
);
CREATE TABLE capture_thread_repair_node (
 repair_id uuid NOT NULL REFERENCES capture_thread_repair(id) ON DELETE RESTRICT,
 activity_id uuid NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
 source_key text NOT NULL,
 message_key text NOT NULL,
 parent_key text NOT NULL DEFAULT '',
 parent_activity_id uuid REFERENCES activity(id) ON DELETE SET NULL,
 own_topic text NOT NULL,
 ancestor_topic text NOT NULL DEFAULT '',
 ancestor_distance integer NOT NULL DEFAULT -1 CHECK(ancestor_distance >= -1),
 frontier_key text NOT NULL DEFAULT '',
 frontier_topic text NOT NULL DEFAULT '',
 root_key text NOT NULL,
 root_proven boolean NOT NULL DEFAULT false,
 graph_pending integer NOT NULL DEFAULT 0 CHECK(graph_pending >= 0),
 graph_state text NOT NULL DEFAULT 'waiting' CHECK(graph_state IN ('waiting','ready','done','cycle')),
 graph_after uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 child_after uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 edges_done boolean NOT NULL DEFAULT false,
 after_seat uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 after_basis uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 after_delivery uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 observed_version bigint NOT NULL,
 repair_after_seat uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 inherit_source text NOT NULL DEFAULT '',
 stage text NOT NULL DEFAULT 'verdict',
 PRIMARY KEY(repair_id,activity_id)
);
CREATE INDEX capture_thread_topic_root ON capture_thread_topic(root_key,activity_id);
CREATE INDEX capture_thread_repair_node_target ON capture_thread_repair_node(repair_id,root_key,activity_id);
CREATE INDEX capture_thread_repair_node_identity ON capture_thread_repair_node(repair_id,message_key);
CREATE INDEX capture_thread_repair_node_parent ON capture_thread_repair_node(repair_id,parent_activity_id,activity_id);
CREATE INDEX capture_thread_repair_node_edges ON capture_thread_repair_node(repair_id,edges_done,activity_id);
CREATE INDEX capture_thread_repair_node_work ON capture_thread_repair_node(repair_id,graph_state,activity_id);
CREATE INDEX capture_thread_repair_node_activity ON capture_thread_repair_node(activity_id,repair_id);
CREATE TABLE capture_thread_invalidation (
 activity_id uuid PRIMARY KEY,
 state text NOT NULL DEFAULT 'ready' CHECK(state IN ('ready','backoff','quarantined')),
 failures integer NOT NULL DEFAULT 0 CHECK(failures BETWEEN 0 AND 3),
 retry_after timestamptz,
 source_key text NOT NULL,
 repair_id uuid REFERENCES capture_thread_repair(id) ON DELETE SET NULL,
 after_repair uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 after_node uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 after_topic uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 proofs_cleared boolean NOT NULL DEFAULT false,
 run_id uuid NOT NULL,
 queue_order bigint GENERATED ALWAYS AS IDENTITY
);
CREATE INDEX capture_thread_invalidation_source ON capture_thread_invalidation(source_key,queue_order);
CREATE INDEX capture_thread_invalidation_fifo ON capture_thread_invalidation(queue_order) WHERE state='ready';
CREATE INDEX capture_thread_invalidation_retry ON capture_thread_invalidation(retry_after,queue_order) WHERE state='backoff';
CREATE INDEX capture_thread_invalidation_quarantine ON capture_thread_invalidation(activity_id) WHERE state='quarantined';
CREATE INDEX capture_thread_invalidation_repair ON capture_thread_invalidation(repair_id,activity_id) WHERE repair_id IS NOT NULL;
CREATE FUNCTION activity_thread_available(thread text) RETURNS boolean
LANGUAGE sql STABLE PARALLEL SAFE AS $$SELECT NOT EXISTS (
 SELECT 1 FROM capture_thread_barrier b JOIN capture_thread_repair r ON r.id=b.repair_id
 WHERE b.blocked_key=thread AND r.phase <> 'published'
) AND NOT EXISTS(SELECT 1 FROM capture_thread_pending p WHERE p.source_key=thread AND p.blocked)
 AND NOT EXISTS(SELECT 1 FROM capture_thread_invalidation i WHERE i.source_key=thread)$$;

