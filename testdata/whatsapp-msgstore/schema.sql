-- The part of WhatsApp Android's msgstore.db that cade reads (#77): the
-- real tables have many more columns, which the reader never selects.
CREATE TABLE jid (_id INTEGER PRIMARY KEY, user TEXT, server TEXT, raw_string TEXT);
CREATE TABLE jid_map (lid_row_id INTEGER PRIMARY KEY, jid_row_id INTEGER, sort_id INTEGER);
CREATE TABLE chat (_id INTEGER PRIMARY KEY, jid_row_id INTEGER, subject TEXT);
CREATE TABLE message (_id INTEGER PRIMARY KEY, chat_row_id INTEGER, from_me INTEGER, key_id TEXT,
	sender_jid_row_id INTEGER, timestamp INTEGER, message_type INTEGER, text_data TEXT);
CREATE TABLE message_media (message_row_id INTEGER PRIMARY KEY, file_path TEXT, media_name TEXT);
