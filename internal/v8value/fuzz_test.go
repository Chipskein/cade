package v8value

import "testing"

// Teams message values are V8-serialized objects written by Chrome; a
// corrupt or unexpected one must fail to decode, never panic or blow the
// stack. Run longer with `make fuzz`.
func FuzzDecode(f *testing.F) {
	f.Add(v8('_'))
	f.Add(v8(append([]byte{'o'}, append(latin1("id"), append(latin1("19:abc"), '{', 1)...)...)...))
	f.Add(v8('A', 2, 'T', 'F', '$', 0, 2))
	f.Add(v8('a', 3, 'I', 2, '-', '@', 0, 3))
	f.Add(v8(';', 'I', 2, 'I', 4, ':', 4))
	f.Fuzz(func(t *testing.T, payload []byte) {
		value, err := Decode(payload)
		if err == nil && value == nil {
			t.Fatalf("Decode(%x) returned neither a value nor an error", payload)
		}
		if value != nil {
			_ = value.String()
			value.Get("content")
		}
	})
}
