package workbench

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"
)

// CyberChefURL starts a private static-file endpoint for the pinned CyberChef
// bundle. The random path is an unguessable capability; the listener is also
// restricted to loopback. Artifact bytes are carried in the browser fragment
// and therefore never enter an HTTP request or server log.
func (a *App) CyberChefURL() (string, error) {
	a.chefMu.Lock()
	defer a.chefMu.Unlock()
	if a.chefBaseURL != "" {
		return a.chefBaseURL, nil
	}
	if a.assets == nil {
		return "", errors.New("Workbench frontend assets are unavailable")
	}
	sub, err := fs.Sub(a.assets, "frontend/dist/vendor/cyberchef")
	if err != nil {
		return "", err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	var tokenBytes [24]byte
	if _, err = rand.Read(tokenBytes[:]); err != nil {
		_ = listener.Close()
		return "", err
	}
	token := hex.EncodeToString(tokenBytes[:])
	prefix := "/" + token + "/"
	files := http.FileServer(http.FS(sub))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method != http.MethodGet && r.Method != http.MethodHead) || !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "private, max-age=3600")
		http.StripPrefix(prefix, files).ServeHTTP(w, r)
	})
	server := &http.Server{Handler: handler}
	a.chefServer = server
	a.chefListener = listener
	a.chefBaseURL = "http://" + listener.Addr().String() + prefix
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			// A failed listener is detected by the frontend load watchdog. Avoid
			// logging the random capability URL or artifact-related information.
		}
	}()
	return a.chefBaseURL, nil
}
