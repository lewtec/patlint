package p
type Box[T any] struct{}
func (b *Box[T]) Helper() int { return 1 }
func (b *Box[T]) Stay() int   { return 2 }
func (t *Plain) Run() int     { return 3 }
