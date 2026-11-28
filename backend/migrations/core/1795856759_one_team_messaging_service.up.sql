CREATE TABLE chat_service (
 id uuid PRIMARY KEY DEFAULT uuidv7(),
 singleton boolean NOT NULL DEFAULT true UNIQUE CHECK (singleton),
 provider text CHECK (provider IN ('dispact','slack','teams')),
 slack_history_profile jsonb,
 version bigint NOT NULL DEFAULT 1 CHECK (version BETWEEN -9007199254740991 AND 9007199254740991),
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO chat_service(provider)
SELECT provider FROM (
 SELECT provider,created_at,id,0 AS priority FROM channel_connection
 WHERE provider IN ('dispact','slack','teams') AND archived_at IS NULL
 UNION ALL SELECT provider,created_at,id,1 AS priority FROM chat_member_grant WHERE status<>'disconnected'
) existing ORDER BY priority,created_at,id LIMIT 1;
GRANT SELECT, INSERT, UPDATE, DELETE ON chat_service TO margince_app;
