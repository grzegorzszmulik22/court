package catalog

// Product is one sellable thing.
type Product struct {
	SKU   string
	Name  string
	Cents int
}

// Index maps SKU to product.
type Index struct{ m map[string]Product }

// New builds an index.
func New() *Index { return &Index{m: map[string]Product{}} }

// Put inserts or replaces.
func (i *Index) Put(p Product) { i.m[p.SKU] = p }

// Get looks up a SKU.
func (i *Index) Get(sku string) (Product, bool) {
	p, ok := i.m[sku]
	return p, ok
}
