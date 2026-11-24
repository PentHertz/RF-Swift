package workbench

import (
	"strings"
	"testing"
)

func TestDetectTransmissionMatchesTransmitModes(t *testing.T) {
	transmit := []string{
		"hackrf_transfer -t captures/gate.cs8 -f 433920000 -s 2000000 -x 10",
		"cd /workspace && hackrf_transfer -f 433920000 -c 64",
		"aireplay-ng --test wlan0mon",
		"timeout 30 mdk4 wlan0mon b",
		"hostapd lab.conf",
		"sudo /usr/bin/hostapd-mana mana.conf",
		"./gnb -c gnb.yaml",
		"srsenb enb.conf",
		"bladeRF-cli -e 'tx config file=x.sc16; tx start'",
		"urh_cli -f 433.92e6 -s 2e6 -tx -file x.complex",
		"d.RFxmit(b'\\x00\\xff')",
		"wifi.deauth all",
		"btlejack -c any -j",
	}
	for _, cmd := range transmit {
		if detectTransmission(cmd) == "" {
			t.Errorf("not detected as transmission: %s", cmd)
		}
	}
	receive := []string{
		"hackrf_transfer -r captures/x.cs8 -f 433920000 -s 2000000 -n 20000000",
		"hackrf_sweep -f 400:470 -r captures/sweep.csv",
		"cat gnb.yaml",
		"vim /etc/hostapd/hostapd.conf",
		"rtl_433 -f 433.92M -F json",
		"airodump-ng --write captures/scan wlan0mon",
		"hostapd_cli status",
		"grep -r aireplay-ng.conf docs",
		"btlejack -s",
		"pm3 -c 'hf search'",
	}
	for _, cmd := range receive {
		if rule := detectTransmission(cmd); rule != "" {
			t.Errorf("receive-only command flagged by %q: %s", rule, cmd)
		}
	}
}

func TestCheckTransmitGate(t *testing.T) {
	tx := "hackrf_transfer -t captures/gate.cs8 -f 433920000 -s 2000000"
	denied := &mcpServer{opts: MCPOptions{AllowExec: true}}
	if _, err := denied.checkTransmit(tx, true, "433.92 MHz, TX VGA 0, 5 s, client gate"); err == nil || !strings.Contains(err.Error(), "Allow RF transmission") {
		t.Fatalf("transmission without permission was not refused: %v", err)
	}
	if _, err := denied.checkTransmit("echo hello", true, "declared"); err == nil {
		t.Fatal("declared transmission without permission was not refused")
	}
	if isTx, err := denied.checkTransmit("rtl_433 -f 433.92M", false, ""); err != nil || isTx {
		t.Fatalf("receive-only command refused: %v %v", isTx, err)
	}
	allowed := &mcpServer{opts: MCPOptions{AllowExec: true, AllowTransmit: true}}
	if _, err := allowed.checkTransmit(tx, false, ""); err == nil || !strings.Contains(err.Error(), "transmits: true") {
		t.Fatalf("undeclared transmission was not refused: %v", err)
	}
	if _, err := allowed.checkTransmit(tx, true, "x"); err == nil {
		t.Fatal("transmission without a description was not refused")
	}
	if isTx, err := allowed.checkTransmit(tx, true, "433.92 MHz, TX VGA 0, 5 s, client gate"); err != nil || !isTx {
		t.Fatalf("declared, described transmission refused: %v %v", isTx, err)
	}
}
