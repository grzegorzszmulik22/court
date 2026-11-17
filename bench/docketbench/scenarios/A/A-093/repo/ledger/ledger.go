package ledger

// Entry is one posting.
type Entry struct {
	Account string
	Cents   int64
}

// Book keeps entries.
type Book struct{ entries []Entry }

// Post appends an entry.
func (b *Book) Post(e Entry) { b.entries = append(b.entries, e) }

// Balance sums one account.
func (b *Book) Balance(acct string) int64 {
	var s int64
	for _, e := range b.entries {
		if e.Account == acct {
			s += e.Cents
		}
	}
	return s
}
