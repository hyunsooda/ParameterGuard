package itf

type Itf interface {
	Get() int
}

type A struct {
	b B
}
type B struct {
	itf Itf
}

func _(i Itf) int { // want "Declared 'i'"
	return i.Get() // want "Unsafely used 'i'"
}

func _(a A) int { // want "Declared 'itf'"
	return a.b.itf.Get() // want "Unsafely used 'itf'"
}
