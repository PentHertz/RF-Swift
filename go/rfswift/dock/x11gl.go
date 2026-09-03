/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package dock

import "runtime"

// glPlatformEnv is read by the images' shell rc (RF-Swift-images
// scripts/corebuild.sh, rfswift_shell_setup): with the value glPlatformEGL the
// shell exports the EGL switches for Qt and SDL and preloads the GLFW shim
// (/usr/lib/rfswift/libglfw-egl.so) so OpenGL tools create their context
// through EGL instead of GLX. Images without the shim ignore the variable.
const (
	glPlatformEnv = "RFSWIFT_GL_PLATFORM"
	glPlatformEGL = "egl"
)

// x11GLEnv returns the environment entries that make OpenGL tools render over
// the X11 display forwarded from this host, or nil when the host's X server
// serves GLX itself.
//
// On macOS the display is XQuartz, whose GLX exposes no fbconfig that Mesa's
// software rasterizer accepts ("No matching fbConfigs or visuals found",
// "glx: failed to create drisw screen"), so every GLX-based tool aborts at
// window creation (SDR++: "OpenGL 3.0 was not supported"). EGL on the same
// display works (llvmpipe, OpenGL 4.5), hence the switch. Linux X servers and
// WSLg serve GLX fine and are left alone.
//
//	out: []string KEY=VALUE entries to add to the container environment
func x11GLEnv() []string {
	return x11GLEnvFor(runtime.GOOS)
}

// x11GLEnvFor is x11GLEnv for a given host OS (testable).
//
//	in(1): string goos runtime.GOOS value of the host
//	out: []string KEY=VALUE entries, nil when nothing is needed
func x11GLEnvFor(goos string) []string {
	if goos != "darwin" {
		return nil
	}
	return []string{glPlatformEnv + "=" + glPlatformEGL}
}

// withX11GLEnv appends x11GLEnv to env unless the caller already set the key
// (an explicit RFSWIFT_GL_PLATFORM from the user or a profile wins).
//
//	in(1): []string env container environment built so far
//	out: []string env with the host's GL entries appended when needed
func withX11GLEnv(env []string) []string {
	if envHasKey(env, glPlatformEnv) {
		return env
	}
	return append(env, x11GLEnv()...)
}
