package msgstore

// requiredTables are the ones messagesQuery reads; the client's other
// tables change between versions and are not needed.
var requiredTables = []string{"message", "chat", "jid", "jid_map", "message_media"}

// messagesQuery returns one row per message, oldest first. A JID may map
// to several LIDs (#77 found a few); the lowest is taken so the same row
// always reads the same.
const messagesQuery = `SELECT m.key_id, m.from_me, m.message_type, m.timestamp, COALESCE(m.text_data, ''),
		cj.raw_string, COALESCE(cl.raw_string, ''), COALESCE(c.subject, ''),
		COALESCE(sj.raw_string, ''), COALESCE(sl.raw_string, ''),
		COALESCE(mm.file_path, mm.media_name, '')
	FROM message m
	JOIN chat c ON c._id = m.chat_row_id
	JOIN jid cj ON cj._id = c.jid_row_id
	LEFT JOIN jid sj ON sj._id = m.sender_jid_row_id
	LEFT JOIN jid cl ON cl._id = (SELECT MIN(lid_row_id) FROM jid_map WHERE jid_row_id = cj._id)
	LEFT JOIN jid sl ON sl._id = (SELECT MIN(lid_row_id) FROM jid_map WHERE jid_row_id = sj._id)
	LEFT JOIN message_media mm ON mm.message_row_id = m._id
	WHERE m.message_type IS NOT NULL AND m.key_id <> '' AND cj.raw_string IS NOT NULL
	ORDER BY m.timestamp, m._id`

// tableExistsQuery tells whether a table is in the file.
const tableExistsQuery = `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`
