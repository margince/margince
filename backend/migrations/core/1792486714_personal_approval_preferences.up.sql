CREATE TABLE approval_preference (
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    kind text NOT NULL,
    muted boolean NOT NULL DEFAULT false,
    PRIMARY KEY (user_id, kind)
);
