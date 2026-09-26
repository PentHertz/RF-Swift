/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package dock

// MIT-SHM across the container boundary. An X client in the container creates
// a SysV shared memory segment in the container's IPC namespace and asks the
// host X server to attach it by id; the server looks the id up in the host's
// namespace. When the host has a segment with that id (small ids are common
// after a reboot), toolkits accept a probe that attached the wrong segment and
// then die on the next MIT-SHM request (GRC: "BadAccess (attempt to access
// private resource denied)", request_code 130). Qt skips MIT-SHM with the
// variable below; the images also preload a library that reports MIT-SHM as
// absent to Xlib clients (GTK, cairo, Mesa), see RF-Swift-images.
const qtNoMITSHMEnv = "QT_X11_NO_MITSHM"

// withX11SHMEnv makes Qt tools skip MIT-SHM on a forwarded X11 display, unless
// the caller already set the variable.
//
//	in(1): []string env container environment built so far
//	out: []string env with QT_X11_NO_MITSHM=1 appended when missing
func withX11SHMEnv(env []string) []string {
	if envHasKey(env, qtNoMITSHMEnv) {
		return env
	}
	return append(env, qtNoMITSHMEnv+"=1")
}
