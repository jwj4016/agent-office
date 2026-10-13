package export

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadInsideStaysInTheDataFolder(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	base := filepath.Join(root, "data")
	os.MkdirAll(filepath.Join(base, "projects"), 0o700)
	os.WriteFile(filepath.Join(base, "projects", "ok.md"), []byte("ok"), 0o600)
	os.WriteFile(filepath.Join(root, "outside.txt"), []byte("secret"), 0o600)
	os.Symlink(filepath.Join(root, "outside.txt"), filepath.Join(base, "projects", "link.md"))

	if b, err := readInside(base, "projects/ok.md"); err != nil || string(b) != "ok" {
		t.Fatalf("inside file: %q %v", b, err)
	}
	for _, rel := range []string{"../outside.txt", "projects/../../outside.txt", "/etc/hosts", "", "projects/link.md", "projects"} {
		if b, err := readInside(base, rel); err == nil {
			t.Fatalf("%q was read: %q", rel, b)
		}
	}
}

func TestMaskKnownAndKeyShapedValues(t *testing.T) {
	m := masker{known: []string{"my-custom-secret-value"}}
	out := string(m.mask([]byte("a my-custom-secret-value b sk-ant-api03-abcdefghijklmnopqrstu c ghp_abcdefghijklmnopqrstuvwxyz d sk-short")))
	want := "a " + Mask + " b " + Mask + " c " + Mask + " d sk-short"
	if out != want || m.count != 3 {
		t.Fatalf("masked %q (%d)", out, m.count)
	}
}
