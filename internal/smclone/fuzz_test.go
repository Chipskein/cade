package smclone

import "testing"

// Firefox IndexedDB values are structured clones written by the browser; a
// corrupt or unexpected one must fail to decode, never panic or blow the
// stack. Run longer with `go tool mage fuzz`.
func FuzzDecode(f *testing.F) {
	f.Add(newClone().pair(tagUndefined, 0).bytes)
	f.Add(newClone().pair(tagObjectObject, 0).latin1("id").twoByte("ção").end().bytes)
	f.Add(newClone().pair(tagArrayObject, 2).int32(1).pair(tagBackReferenceObject, 0).end().bytes)
	f.Add(newClone().pair(tagTypedArrayObject, 1).word(1).pair(tagArrayBufferObject, 0).word(1).padded([]byte{9}).word(0).bytes)
	f.Add(newClone().pair(tagDOMFile, 0).word(1).lengthPrefixed("a/b").word(0).lengthPrefixed("n").bytes)
	f.Fuzz(func(t *testing.T, payload []byte) {
		value, err := Decode(payload)
		if err == nil && value == nil {
			t.Fatalf("Decode(%x) returned neither a value nor an error", payload)
		}
		if value != nil {
			_ = value.String()
			value.Get("id")
		}
	})
}
