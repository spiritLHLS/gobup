package services

import (
	"os"
	"strings"
	"testing"

	"github.com/gobup/server/internal/models"
)

func TestResolveDanmakuBurnOptions(t *testing.T) {
	options := resolveDanmakuBurnOptions(&models.RecordRoom{
		DanmakuBurnStyle:   "large",
		DanmakuFontSize:    18,
		DanmakuFontColor:   "#3366CC",
		DanmakuScrollArea:  1.5,
		DanmakuDisplayArea: -1,
	})

	if options.FontSize != 18 {
		t.Fatalf("FontSize = %d, want 18", options.FontSize)
	}
	if options.PrimaryColorASS != "&H00CC6633" {
		t.Fatalf("PrimaryColorASS = %q, want &H00CC6633", options.PrimaryColorASS)
	}
	if options.ScrollArea != 1 {
		t.Fatalf("ScrollArea = %v, want 1", options.ScrollArea)
	}
	if options.DisplayArea != 0.8 {
		t.Fatalf("DisplayArea = %v, want 0.8", options.DisplayArea)
	}
}

func TestApplyASSStyleOverrides(t *testing.T) {
	tmp, err := os.CreateTemp("", "gobup_ass_style_*.ass")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	ass := `[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour
Style: R2L,WenQuanYi Zen Hei,38,&H00FFFFFF,&H000000FF

[Events]
`
	if _, err := tmp.WriteString(ass); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}

	err = applyASSStyleOverrides(tmp.Name(), danmakuBurnOptions{
		FontSize:        24,
		PrimaryColorASS: "&H00112233",
	})
	if err != nil {
		t.Fatalf("applyASSStyleOverrides() error = %v", err)
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	updated := string(data)
	if !strings.Contains(updated, "Style: R2L,WenQuanYi Zen Hei,24,&H00112233,&H000000FF") {
		t.Fatalf("ASS style not updated:\n%s", updated)
	}
}

func TestNormalizeASSPrimaryColorRejectsInvalid(t *testing.T) {
	if got := normalizeASSPrimaryColor("not-a-color"); got != "" {
		t.Fatalf("normalizeASSPrimaryColor invalid = %q, want empty", got)
	}
	if got := normalizeASSPrimaryColor("&H00ABCDEF"); got != "&H00ABCDEF" {
		t.Fatalf("normalizeASSPrimaryColor ASS = %q", got)
	}
}
