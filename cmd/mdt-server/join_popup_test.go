package main

import (
	"strings"
	"testing"

	"mdt-server/internal/config"
)

func TestRenderJoinPopupValueReplacesPlaceholders(t *testing.T) {
	runtimeCfg := joinPopupRuntimeConfig{
		Config: config.JoinPopupConfig{
			LinkURL: "https://example.com/rules",
		},
		ServerName:     "测试服",
		VirtualPlayers: 7,
	}

	got := renderJoinPopupValue(runtimeCfg, nil, nil, "{server_name}|{player_name}|{current_map}|{players}|{link_url}")
	want := "测试服|玩家|unknown|7|https://example.com/rules"
	if got != want {
		t.Fatalf("expected rendered popup placeholders %q, got %q", want, got)
	}
}

func TestBuildJoinPopupMessageCombinesIntroAndAnnouncement(t *testing.T) {
	runtimeCfg := joinPopupRuntimeConfig{
		Config: config.JoinPopupConfig{
			Message:          "欢迎 [green]{player_name}[]",
			AnnouncementText: "[accent]当前地图[] {current_map}",
		},
	}

	got := buildJoinPopupMessage(runtimeCfg, nil, nil)
	want := "欢迎 [green]玩家[]\n\n[accent]当前地图[] unknown"
	if got != want {
		t.Fatalf("expected combined popup message %q, got %q", want, got)
	}
}

func TestBuildJoinPopupMessageSkipsBlankSections(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.JoinPopupConfig
		wantMsg string
	}{
		{
			name: "only intro",
			cfg: config.JoinPopupConfig{
				Message: "欢迎玩家",
			},
			wantMsg: "欢迎玩家",
		},
		{
			name: "only announcement",
			cfg: config.JoinPopupConfig{
				AnnouncementText: "服务器公告",
			},
			wantMsg: "服务器公告",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildJoinPopupMessage(joinPopupRuntimeConfig{Config: tc.cfg}, nil, nil)
			if got != tc.wantMsg {
				t.Fatalf("expected popup message %q, got %q", tc.wantMsg, got)
			}
		})
	}
}

func TestHelpPagesUseButtonLayout(t *testing.T) {
	runtimeCfg := joinPopupRuntimeConfig{
		Config: config.JoinPopupConfig{
			HelpText: "[accent]欢迎查看帮助[]",
		},
	}

	pages := helpPages(runtimeCfg, nil, nil)
	if len(pages) != 2 {
		t.Fatalf("expected 2 help pages, got %d", len(pages))
	}
	if !strings.Contains(pages[0].Message, "[accent]欢迎查看帮助[]") {
		t.Fatalf("expected help intro on first page, got %q", pages[0].Message)
	}
	if len(pages[0].Buttons) != 10 {
		t.Fatalf("expected first help page to expose 10 command buttons, got %d", len(pages[0].Buttons))
	}
	if len(pages[1].Buttons) != 5 {
		t.Fatalf("expected second help page to expose remaining 5 command buttons, got %d", len(pages[1].Buttons))
	}
	for _, want := range []string{"/help\n打开帮助", "/status\n查看状态", "/sync\n重新同步", "/vote\n投票页面"} {
		found := false
		for _, button := range pages[0].Buttons {
			if button.Label == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected first help page to contain button %q, got %+v", want, pages[0].Buttons)
		}
	}
}

func TestHelpPageOptionsExposeNavigationAndActions(t *testing.T) {
	pages := helpPages(joinPopupRuntimeConfig{Config: config.Default().JoinPopup}, nil, nil)
	first := helpPageOptions(pages[0], 0, len(pages))
	if len(first) != 11 {
		t.Fatalf("expected first page to contain 10 command rows plus nav row, got %#v", first)
	}
	for i := 0; i < 10; i++ {
		if len(first[i]) != 1 {
			t.Fatalf("expected help command row %d to contain one button, got %#v", i, first[i])
		}
	}
	if got := first[10]; len(got) != 3 || got[0] != "[gray]上一页[]" || got[1] != "关闭" || got[2] != "下一页" {
		t.Fatalf("unexpected first-page help options: %#v", first)
	}

	last := helpPageOptions(pages[1], 1, len(pages))
	if len(last) != 6 {
		t.Fatalf("expected last page to contain 5 command rows plus nav row, got %#v", last)
	}
	for i := 0; i < 5; i++ {
		if len(last[i]) != 1 {
			t.Fatalf("expected trailing help command row %d to contain one button, got %#v", i, last[i])
		}
	}
	if got := last[5]; len(got) != 3 || got[0] != "上一页" || got[1] != "关闭" || got[2] != "[gray]下一页[]" {
		t.Fatalf("unexpected last-page help options: %#v", last)
	}
}
