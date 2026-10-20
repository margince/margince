SET LOCAL lock_timeout = '3s';

DROP TABLE data_task_run;
UPDATE role SET permissions = permissions #- '{objects,data_task}' WHERE is_system AND permissions->'objects' ? 'data_task';
