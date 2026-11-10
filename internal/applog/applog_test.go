package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitPrunesToTenSessions(t *testing.T) {
	dir := t.TempDir()
	SetDirForTest(dir)
	defer ResetForTest()

	for i := 1; i <= 12; i++ {
		name := filepath.Join(dir, fmt.Sprintf("session-20260101-1200%02d.log", i))
		if err := os.WriteFile(name, []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := Init(); err != nil {
		t.Fatal(err)
	}
	defer Close()

	matches, err := filepath.Glob(filepath.Join(dir, "session-*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != keepSessions {
		t.Fatalf("kept %d files, want %d: %v", len(matches), keepSessions, matches)
	}
}

func TestCmdWritesOutput(t *testing.T) {
	dir := t.TempDir()
	SetDirForTest(dir)
	defer ResetForTest()

	if err := Init(); err != nil {
		t.Fatal(err)
	}
	defer Close()

	Cmd("az", []string{"account", "show"}, "subscription: demo\n", nil)
	Cmd("az", []string{"fail"}, "boom", os.ErrPermission)

	Close()

	matches, err := filepath.Glob(filepath.Join(dir, "session-*.log"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("matches=%v err=%v", matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"$ az account show",
		"subscription: demo",
		"exit: ok",
		"$ az fail",
		"boom",
		"exit: error:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("log missing %q in:\n%s", want, text)
		}
	}
}

func TestInfofNoopWithoutInit(t *testing.T) {
	ResetForTest()
	Infof("should not panic %d", 1)
	Cmd("x", nil, "", nil)
}

func TestWriterDiscardWithoutInit(t *testing.T) {
	ResetForTest()
	if Writer() == nil {
		t.Fatal("expected non-nil writer")
	}
}

func TestWriterReturnsSessionFile(t *testing.T) {
	dir := t.TempDir()
	SetDirForTest(dir)
	defer ResetForTest()
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	defer Close()
	w := Writer()
	if _, err := w.Write([]byte("redirected\n")); err != nil {
		t.Fatal(err)
	}
}

func TestRecentCmdsFromCmd(t *testing.T) {
	ResetForTest()
	defer ResetForTest()
	Cmd("az", []string{"account", "set", "--subscription", "sub"}, "", nil)
	Cmd("kubelogin", []string{"convert-kubeconfig", "-l", "azurecli"}, "", nil)
	got := RecentCmds()
	if len(got) != 2 {
		t.Fatalf("RecentCmds = %v", got)
	}
	if got[0] != "az account set --subscription sub" {
		t.Fatalf("first = %q", got[0])
	}
	if got[1] != "kubelogin convert-kubeconfig -l azurecli" {
		t.Fatalf("second = %q", got[1])
	}
}
