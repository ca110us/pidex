package api

import (
	"testing"

	"github.com/google/uuid"

	"pidex/internal/store/dbq"
)

func TestAnswersShowOnlyThePhotosTheyCite(t *testing.T) {
	photos := make([]dbq.Photo, 12)
	for i := range photos {
		photos[i].ID = uuid.New()
	}
	answer, kept := cited("没有黑色的猫。它们只是在黑色衣物上休息[7][2]，也睡在包里[9][2][30]。", photos)
	if answer != "没有黑色的猫。它们只是在黑色衣物上休息[1][2]，也睡在包里[3][2]。" {
		t.Errorf("answer = %q", answer)
	}
	if len(kept) != 3 || kept[0].ID != photos[6].ID || kept[1].ID != photos[1].ID || kept[2].ID != photos[8].ID {
		t.Errorf("kept %d photos, not [7] [2] [9] in that order", len(kept))
	}
	// An answer from the register alone shows none.
	if answer, kept := cited("共有四只猫。", photos); answer != "共有四只猫。" || len(kept) != 0 {
		t.Errorf("uncited answer: %q with %d photos", answer, len(kept))
	}
}

func TestLooksAreOneLine(t *testing.T) {
	if got := looksOf("黑白双色毛发。\n\n鼻子呈粉红色。\n"); got != "黑白双色毛发; 鼻子呈粉红色" {
		t.Errorf("looks = %q", got)
	}
}
