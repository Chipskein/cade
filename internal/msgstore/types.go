package msgstore

// MessageType is the Android client's message_type column. The values are
// not documented; the ones named here were classified on a real msgstore.db
// (#77) by the tables that hold their parts (message_system,
// message_media, message_revoked, message_call_log…) and by which of them
// WhatsApp Web keeps as messages.
type MessageType int

const (
	TypeText           MessageType = 0
	TypeImage          MessageType = 1
	TypeAudio          MessageType = 2
	TypeVideo          MessageType = 3
	TypeContact        MessageType = 4
	TypeLocation       MessageType = 5
	TypeSystem         MessageType = 7
	TypeDocument       MessageType = 9
	TypeGIF            MessageType = 13
	TypeRevoked        MessageType = 15
	TypeLiveLocation   MessageType = 16
	TypeSticker        MessageType = 20
	TypeViewOnceImage  MessageType = 42
	TypeViewOnceVideo  MessageType = 43
	TypeAdminRevoked   MessageType = 64
	TypePoll           MessageType = 66
	TypeCall           MessageType = 90
	TypeAlbum          MessageType = 99
	typeMediaVariantA  MessageType = 25
	typeLinkedText     MessageType = 27
	typeMediaVariantB  MessageType = 28
	typeMediaVariantC  MessageType = 57
	typeCaptionedMedia MessageType = 62
)

// contentTypes are written by a person; every other type is kept only
// when it carries text, and never when it is a system notice.
var contentTypes = map[MessageType]bool{
	TypeText: true, TypeImage: true, TypeAudio: true, TypeVideo: true, TypeContact: true,
	TypeLocation: true, TypeDocument: true, TypeGIF: true, TypeLiveLocation: true, TypeSticker: true,
	TypeViewOnceImage: true, TypeViewOnceVideo: true, TypePoll: true, TypeAlbum: true,
	typeMediaVariantA: true, typeLinkedText: true, typeMediaVariantB: true, typeMediaVariantC: true, typeCaptionedMedia: true,
}

// IsMessage reports whether a row of this type with this text is a
// message someone wrote, rather than a system notice, a call or a
// deleted message.
//
//	msgstore.TypeImage.IsMessage("") // true: a photo without caption
func (t MessageType) IsMessage(text string) bool {
	if contentTypes[t] {
		return true
	}
	return t != TypeSystem && text != ""
}
