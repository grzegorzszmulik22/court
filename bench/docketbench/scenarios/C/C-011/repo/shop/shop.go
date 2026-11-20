package shop

// Item is one line in a cart.
type Item struct {
	Name  string
	Cents int
	Qty   int
}

// Total sums the cart.
func Total(items []Item) int {
	s := 0
	for _, it := range items {
		s += it.Cents * it.Qty
	}
	return s
}

// Discount applies a percentage off.
func Discount(cents, pct int) int {
	return cents - cents*clamp(pct, 0, 100)/100
}
