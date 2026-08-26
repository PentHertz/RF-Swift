package integration_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"penthertz/rfswift-workbench/internal/workbench"
)

func TestCyberChefLoopbackServer(t *testing.T) {
	app := workbench.NewApp(os.DirFS("../.."))
	base, err := app.CyberChefURL()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Shutdown(context.Background()) })
	if !strings.HasPrefix(base, "http://127.0.0.1:") || !strings.HasSuffix(base, "/") {
		t.Fatalf("unsafe CyberChef base URL %q", base)
	}
	res, err := http.Get(base + "CyberChef_v11.4.0.html")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "CyberChef") {
		t.Fatalf("CyberChef response = %d %q", res.StatusCode, body)
	}
	res, err = http.Get(strings.TrimSuffix(base, "/") + "-wrong/CyberChef_v11.4.0.html")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("wrong token returned %d", res.StatusCode)
	}
}
