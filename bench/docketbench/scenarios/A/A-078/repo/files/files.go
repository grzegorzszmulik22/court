package files

// Record is one indexed file.
type Record struct {
	Path  string
	Bytes int64
}

// Index is a set of records.
type Index struct{ recs []Record }

// Add appends a record.
func (i *Index) Add(r Record) { i.recs = append(i.recs, r) }

// Size sums bytes.
func (i *Index) Size() int64 {
	var s int64
	for _, r := range i.recs {
		s += r.Bytes
	}
	return s
}
