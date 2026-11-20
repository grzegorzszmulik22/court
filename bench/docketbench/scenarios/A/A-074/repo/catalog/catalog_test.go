package catalog

import "testing"

func TestPutGet(t *testing.T) {
	i := New()
	i.Put(Product{SKU: "x1", Name: "Widget", Cents: 999})
	p, ok := i.Get("x1")
	if !ok || p.Cents != 999 {
		t.Fatalf("Get = %v %v", p, ok)
	}
}

func TestMissing(t *testing.T) {
	i := New()
	if _, ok := i.Get("nope"); ok {
		t.Fatal("found missing sku")
	}
}
