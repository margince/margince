UPDATE role SET permissions = permissions #- '{objects,personal_channel}'
    WHERE permissions ? 'objects' AND (permissions -> 'objects') ? 'personal_channel';
DROP TABLE IF EXISTS personal_channel_upload;
DROP TABLE IF EXISTS personal_channel_reply;
DROP TABLE IF EXISTS personal_channel_connection;
