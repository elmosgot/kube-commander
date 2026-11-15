package ui

import (
	"strings"
	"testing"
)

func TestRenderHelpContextSensitive(test *testing.T) {
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

	for _, testcase := range cases {
		test.Run(testcase.name, func(t *testing.T) {
			model := base
			model.focus = testcase.focus
			model.rightView = testcase.view
			model.logFilterActive = testcase.filter
			if testcase.filter {
				model.logFilter = tErr
			}
			help := model.renderHelp()
			for _, w := range testcase.want {
				if !strings.Contains(help, w) {
					t.Fatalf("help missing %q:\n%s", w, help)
				}
			}
			for _, w := range testcase.notWant {
				if strings.Contains(help, w) {
					t.Fatalf("help unexpectedly has %q:\n%s", w, help)
				}
			}
		})
	}
}
