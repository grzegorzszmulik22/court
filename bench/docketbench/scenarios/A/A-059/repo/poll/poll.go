package poll

// Vote is one ballot.
type Vote struct {
	Choice string
	Weight int
}

// Tally counts weighted votes per choice.
func Tally(vs []Vote) map[string]int {
	out := map[string]int{}
	for _, v := range vs {
		w := v.Weight
		if w <= 0 {
			w = 1
		}
		out[v.Choice] += w
	}
	return out
}
