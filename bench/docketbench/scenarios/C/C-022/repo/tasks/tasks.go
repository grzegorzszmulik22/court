package tasks

// Task is one tracked unit of work.
type Task struct {
	ID    int
	Title string
	Done  bool
}

// List is an ordered set of tasks.
type List struct {
	items []Task
	next  int
}

// Add appends a task and returns its id.
func (l *List) Add(title string) int {
	l.next++
	l.items = append(l.items, Task{ID: l.next, Title: title})
	return l.next
}

// Open counts unfinished tasks.
func (l *List) Open() int {
	n := 0
	for _, it := range l.items {
		if !it.Done {
			n++
		}
	}
	return n
}
