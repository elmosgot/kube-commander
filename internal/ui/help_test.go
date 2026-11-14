package ui

import (
	"strings"
	"testing"
)

func TestRenderHelpContextSensitive(t *testing.T) {
	base := New(testConfig(), "test.yaml")
	base.width = 200

	cases := []struct {
		name    string
		focus   pane
		view    rightView
		filter  bool
		want    []string
		notWant []string
	}{
		{
			name:    "left pane",
			focus:   paneLeft,
			view:    viewNamespaces,
			want:    []string{helpAzLogin, helpEnsure, helpEnterOpen},
			notWant: []string{tFilterTok, helpLogs, helpShell, "backspace"},
		},
		{
			name:    "right namespaces",
			focus:   paneRight,
			view:    viewNamespaces,
			want:    []string{helpEnterOpen, helpBackspaceBack},
			notWant: []string{helpAzLogin, helpEnsure, helpLogs, tFilterTok},
		},
		{
			name:    "pod detail",
			focus:   paneRight,
			view:    viewPodDetail,
			want:    []string{helpLogs, helpShell, helpBackspaceBack},
			notWant: []string{helpAzLogin, tFilterTok, helpEnterOpen},
		},
		{
			name:    "pod logs",
			focus:   paneRight,
			view:    viewPodLogs,
			want:    []string{helpFilter, "y copy", "ctrl+click copy", "r restart follow", helpBackspaceBack},
			notWant: []string{helpAzLogin, helpLogs, helpShell, helpEnterOpen, "r refresh"},
		},
		{
			name:    "filter active",
			focus:   paneRight,
			view:    viewPodLogs,
			filter:  true,
			want:    []string{"Filter:", "esc clear"},
			notWant: []string{"tab focus", "c commands"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base
			m.focus = tc.focus
			m.rightView = tc.view
			m.logFilterActive = tc.filter
			if tc.filter {
				m.logFilter = tErr
			}
			help := m.renderHelp()
			for _, w := range tc.want {
				if !strings.Contains(help, w) {
					t.Fatalf("help missing %q:\n%s", w, help)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(help, w) {
					t.Fatalf("help unexpectedly has %q:\n%s", w, help)
				}
			}
		})
	}
}
