package copypresentation

import (
	"strings"
	"testing"
)

func TestNamesAndCategoriesAreBoundedProductLabels(t *testing.T) {
	for _, s := range []string{"", "A new label", "中文名称", "tool & docs", strings.Repeat("字", 160)} {
		if !ValidName(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{" leading", "trailing ", "<script>", "x\nname", "a\u202eb", "\ufffd", string([]byte{255}), strings.Repeat("a", 161), strings.Repeat("字", 161)} {
		if ValidName(s) {
			t.Fatalf("unsafe label accepted %q", s)
		}
	}
	for _, s := range []string{"AUTO", "SOFTWARE", "VIDEO", "DOCUMENT", "IMAGE", "AUDIO", "ARCHIVE", "OTHER"} {
		if !ValidCategory(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"", "CUSTOM", "document", "<script>"} {
		if ValidCategory(s) {
			t.Fatal(s)
		}
	}
	if !(Overlay{Category: "AUTO"}).Valid() || (Overlay{Category: "AUTO", Hidden: true}).Valid() || (Overlay{Category: "AUTO", Version: 1}).Valid() {
		t.Fatal("missing overlay confused with persisted version")
	}
}
