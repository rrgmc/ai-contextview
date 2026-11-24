package sync

// Retrier retries operations.
type Retrier struct {
	n    int
	base int
}

// Next returns the next delay.
func (r *Retrier) Next() int {
	d := r.base << r.n
	r.n++
	return d
}

// Reset resets the retrier.
func (r Retrier) Reset() Retrier {
	r.n = 0
	return r
}

// New creates a Retrier.
func New(base int) *Retrier {
	return &Retrier{base: base}
}

// List is a generic list.
type List[T any] struct {
	items []T
}

// Push appends v.
func (l *List[T]) Push(v T) {
	l.items = append(l.items, v)
}

// MaxDelay caps the delay.
const MaxDelay = 30

var (
	// DefaultBase is the default base delay.
	DefaultBase = 1
	other       = 2
)
