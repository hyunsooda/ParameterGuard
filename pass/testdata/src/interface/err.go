package itf

type Itf interface {
	Get() int
}

func _(i Itf) int { // want "Declared 'i'"
	return i.Get() // want "Unsafely used 'i'"
}
