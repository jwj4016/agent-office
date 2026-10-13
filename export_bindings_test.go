package main

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestExportProjectBinding(t *testing.T) {
	a := startApp(t)
	p, _ := buildService(t, a, "게임: 출시/1차")
	dir := t.TempDir()
	var asked runtime.SaveDialogOptions
	a.saveDialog = func(_ context.Context, o runtime.SaveDialogOptions) (string, error) {
		asked = o
		return filepath.Join(dir, "내보내기"), nil // no extension typed
	}
	res := need(a.ExportProject(p.ID))
	if res.Path != filepath.Join(dir, "내보내기.zip") || res.Rows == 0 || res.ProjectName != p.Name {
		t.Fatalf("result = %+v", res)
	}
	if asked.DefaultFilename != exportFileName(p.Name, time.Now()) || !strings.HasPrefix(asked.DefaultFilename, "게임_ 출시_1차-내보내기-") {
		t.Fatalf("default name = %q", asked.DefaultFilename)
	}
	zr, err := zip.OpenReader(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	zr.Close()
	left, _ := os.ReadDir(dir)
	if len(left) != 1 {
		t.Fatalf("temporary files left: %v", left)
	}

	// Cancelling the dialog writes nothing.
	a.saveDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) { return "", nil }
	if res := need(a.ExportProject(p.ID)); res.Path != "" {
		t.Fatalf("cancelled export = %+v", res)
	}
	if _, err := a.ExportProject("prj-nope"); err == nil {
		t.Fatal("unknown project exported")
	}
}
