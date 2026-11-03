SET LOCAL lock_timeout = '3s';
CREATE TABLE capture_party_source (
 activity_id uuid NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
 user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
 provider text NOT NULL,
 raw_capture_id uuid REFERENCES raw_capture(id) ON DELETE SET NULL,
 sent_by_owner boolean NOT NULL,
 containers text[] NOT NULL,
 containers_known boolean NOT NULL,
 recipients text[] NOT NULL,
 policy_addresses text[] NOT NULL,
 admission_blocked boolean NOT NULL,
 admission_pending boolean NOT NULL DEFAULT true,
 admission_generation bigint NOT NULL DEFAULT 1,
 PRIMARY KEY (activity_id, user_id, provider)
);
CREATE INDEX capture_party_source_original ON capture_party_source(raw_capture_id) WHERE raw_capture_id IS NOT NULL;
CREATE INDEX capture_party_source_recipients ON capture_party_source USING gin(recipients) WHERE sent_by_owner;
CREATE TABLE capture_party_admission_cursor (
 singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
 after_activity uuid NOT NULL,
 after_user uuid NOT NULL,
 ceiling uuid NOT NULL
);
CREATE INDEX capture_party_source_user ON capture_party_source(user_id);
CREATE TABLE capture_party_source_replay (
 activity_id uuid PRIMARY KEY REFERENCES activity(id) ON DELETE CASCADE,
 outcome text NOT NULL
);

CREATE INDEX capture_party_source_pending ON capture_party_source(activity_id,user_id) WHERE admission_pending;
ALTER TABLE comms_outbound ADD COLUMN party_admission_pending boolean NOT NULL DEFAULT true;
CREATE INDEX comms_outbound_party_admission ON comms_outbound(activity_id,user_id) WHERE party_admission_pending AND status='sent' AND sender_kind='user' AND channel_user_id IS NULL;
