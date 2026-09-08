package dock

import "testing"

func TestNormalizeGPUSpec(t *testing.T) {
	for in, want := range map[string]string{
		"all": "all", " All ": "all", "all (amd)": "all", "nvidia (fallback)": "all", "amd": "all",
		"0,1": "0,1", " 0 , 1 ": "0,1", "GPU-abc": "GPU-abc",
		"": "", "none": "", "off": "",
	} {
		if got := NormalizeGPUSpec(in); got != want {
			t.Errorf("NormalizeGPUSpec(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSerialSummary(t *testing.T) {
	hotplug, ports := serialSummary([]string{"c 189:* rwm"}, []string{"/dev/ttyACM0"})
	if hotplug || len(ports) != 0 {
		t.Errorf("without the serial rules: %v %v", hotplug, ports)
	}
	hotplug, ports = serialSummary([]string{"c 189:* rwm", " c 166:* rwm"}, []string{"/dev/ttyACM0", "/dev/ttyUSB0"})
	if !hotplug || len(ports) != 2 {
		t.Errorf("with the serial rules: %v %v", hotplug, ports)
	}
	if _, ports = serialSummary(SerialCgroupRules, nil); ports == nil {
		t.Error("ports must be an array, not null, for the GUI")
	}
}

func TestNormalizeCapName(t *testing.T) {
	for in, want := range map[string]string{"NET_ADMIN": "NET_ADMIN", "CAP_NET_ADMIN": "NET_ADMIN", " cap_sys_rawio ": "SYS_RAWIO", "": ""} {
		if got := NormalizeCapName(in); got != want {
			t.Errorf("NormalizeCapName(%q) = %q", in, got)
		}
	}
}
