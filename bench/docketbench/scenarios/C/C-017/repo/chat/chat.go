package chat

// Message is one line in a room.
type Message struct {
	From string
	Text string
	Seq  int
}

// Room holds messages.
type Room struct{ msgs []Message }

// Say appends.
func (r *Room) Say(from, text string) {
	r.msgs = append(r.msgs, Message{From: from, Text: text, Seq: len(r.msgs) + 1})
}

// Count is the number of messages.
func (r *Room) Count() int { return len(r.msgs) }
