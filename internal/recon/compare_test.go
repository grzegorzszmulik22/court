package recon

// C2.3.2 — the §7.1 comparison over the frozen fixture pairs.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"court/internal/ledger"
)

type vector struct {
	Name  string            `json:"name"`
	Orch  []ledger.Expected `json:"orchestrator"`
	Clerk []ledger.Expected `json:"clerk"`
	Want  struct {
		Both      [][2]int `json:"both"`
		OnlyOrch  []int    `json:"only_orch"`
		OnlyClerk []int    `json:"only_clerk"`
	} `json:"expect"`
}

func TestC2_3_2(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "vectors", "clerk")
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) < 40 {
		t.Fatalf("want >=40 clerk vectors in %s, got %d", dir, len(files))
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var v vector
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		got := Compare(v.Orch, v.Clerk)
		if got.OnlyOrch == nil {
			got.OnlyOrch = []int{}
		}
		if got.OnlyClerk == nil {
			got.OnlyClerk = []int{}
		}
		if got.Both == nil {
			got.Both = [][2]int{}
		}
		if !reflect.DeepEqual(got.Both, v.Want.Both) ||
			!reflect.DeepEqual(got.OnlyOrch, v.Want.OnlyOrch) ||
			!reflect.DeepEqual(got.OnlyClerk, v.Want.OnlyClerk) {
			t.Errorf("%s: got %+v want %+v", v.Name, got, v.Want)
		}
	}
}
