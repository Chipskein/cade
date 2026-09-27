package rootfs

import "testing"

func TestNameStripsTheRoot(t *testing.T) {
	cases := map[string]string{"/": ".", "/home/me": "home/me", "/home/me/../you/": "home/you"}
	for path, want := range cases {
		if got := Name(path); got != want {
			t.Errorf("Name(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestPathInvertsName(t *testing.T) {
	for _, path := range []string{"/", "/home/me/.config"} {
		if got := Path(Name(path)); got != path {
			t.Errorf("Path(Name(%q)) = %q", path, got)
		}
	}
}

func TestNameResolvesRelativePaths(t *testing.T) {
	if got := Name("notes"); got == "notes" || got[0] == '/' {
		t.Fatalf("expected a name under the working directory, got %q", got)
	}
}
