package nix

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSiblingCatalogMatchesEmbedded(t *testing.T) {
	sibling := filepath.Join("..", "..", "..", "..", "RF-Swift-nix", "catalog.json")
	b, err := os.ReadFile(sibling)
	if os.IsNotExist(err) {
		t.Skip("no companion checkout")
	}
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := os.ReadFile("catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(embedded, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatal("embedded catalog differs from RF-Swift-nix; synchronize the catalog before release")
	}
}
