package workbench

import (
	"strings"
	"testing"
)

func TestRecommendToolsForIQCapture(t *testing.T) {
	got, err := recommendRFSwiftTools("inspect and demodulate a radio signal", "door.iq", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || (got[0].Environment != "sdr_light" && got[0].Environment != "sdr_full") {
		t.Fatalf("unexpected IQ recommendations: %#v", got)
	}
	joined := strings.Join(got[0].Tools, " ") + " " + strings.Join(got[0].Examples, " ")
	if !strings.Contains(joined, "inspectrum") && !strings.Contains(joined, "urh") {
		t.Fatalf("IQ inspection tools missing: %#v", got[0])
	}
}

func TestRecommendToolsForMIFAREBadge(t *testing.T) {
	got, err := recommendRFSwiftTools("assess a MIFARE NFC badge", "card.eml", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].Environment != "rfid" {
		t.Fatalf("unexpected RFID recommendations: %#v", got)
	}
	if !strings.Contains(strings.Join(got[0].Examples, " "), "pm3") {
		t.Fatalf("Proxmark command examples missing: %#v", got[0])
	}
}

func TestRecommendToolsRejectsEmptyTask(t *testing.T) {
	if _, err := recommendRFSwiftTools("", "", 4); err == nil {
		t.Fatal("expected an empty-query error")
	}
}
