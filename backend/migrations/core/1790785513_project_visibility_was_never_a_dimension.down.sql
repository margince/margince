SET LOCAL lock_timeout = '3s';

ALTER TABLE project ADD COLUMN visibility text NOT NULL DEFAULT 'workspace';
ALTER TABLE project ADD CONSTRAINT project_visibility_check CHECK (visibility = 'workspace');
