package dock

import "testing"

func TestWithX11SHMEnv(t *testing.T) {
	got := withX11SHMEnv([]string{"DISPLAY=:0"})
	if len(got) != 2 || got[1] != "QT_X11_NO_MITSHM=1" {
		t.Fatalf("QT_X11_NO_MITSHM not added: %v", got)
	}
	keep := withX11SHMEnv([]string{"DISPLAY=:0", "QT_X11_NO_MITSHM=0"})
	if len(keep) != 2 || keep[1] != "QT_X11_NO_MITSHM=0" {
		t.Fatalf("an explicit value was overridden: %v", keep)
	}
}

func TestCombineEnvX11AddsNoMITSHM(t *testing.T) {
	if !envHasKey(combineEnv("DISPLAY=:0", "tcp:127.0.0.1:34567", ""), qtNoMITSHMEnv) {
		t.Fatal("an X11 container misses QT_X11_NO_MITSHM")
	}
	if envHasKey(combineEnv("", "tcp:127.0.0.1:34567", ""), qtNoMITSHMEnv) {
		t.Fatal("a container without X11 got QT_X11_NO_MITSHM")
	}
}
