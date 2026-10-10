package card

// Opt is an optional value. The zero value is empty.
//
// Empty means unknown, and unknown never equals anything, including another
// empty (AGENTS.md hard rule 3). Opt is deliberately not comparable with ==
// (the zero-length func array makes == a compile error), so the only way to
// compare two values is Equal, which enforces that rule.
type Opt[T comparable] struct {
	_  [0]func()
	v  T
	ok bool
}

// Some returns a set value.
func Some[T comparable](v T) Opt[T] { return Opt[T]{v: v, ok: true} }

// None returns an empty value. It is the same as the zero value.
func None[T comparable]() Opt[T] { return Opt[T]{} }

// Get returns the value and whether it is set.
func (o Opt[T]) Get() (T, bool) { return o.v, o.ok }

// IsEmpty reports whether the value is unknown.
func (o Opt[T]) IsEmpty() bool { return !o.ok }

// Equal is true only when both values are set and equal.
func (o Opt[T]) Equal(p Opt[T]) bool { return o.ok && p.ok && o.v == p.v }
