package dock

import "testing"

func TestX11GLEnvForOnlyDarwinGetsEGL(t *testing.T) {
	if got := x11GLEnvFor("darwin"); len(got) != 1 || got[0] != "RFSWIFT_GL_PLATFORM=egl" {
		t.Fatalf("darwin: got %v, want [RFSWIFT_GL_PLATFORM=egl]", got)
	}
	for _, goos := range []string{"linux", "windows", "freebsd"} {
		if got := x11GLEnvFor(goos); got != nil {
			t.Fatalf("%s: got %v, want nil", goos, got)
		}
	}
}

func TestWithX11GLEnvKeepsExplicitValue(t *testing.T) {
	env := []string{"DISPLAY=:0", "RFSWIFT_GL_PLATFORM=glx"}
	got := withX11GLEnv(env)
	if len(got) != 2 || got[1] != "RFSWIFT_GL_PLATFORM=glx" {
		t.Fatalf("explicit value overridden: %v", got)
	}
}
