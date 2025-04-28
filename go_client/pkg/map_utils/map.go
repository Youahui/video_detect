package map_utils

import (
	"sync"
	"sync/atomic"
)

type Map[K comparable, V any] struct {
	m sync.Map
}

func (v *Map[K, V]) Assert(value any, exist bool) (V, bool) {
	if !exist {
		var empty V
		return empty, false
	}
	assertVal, ok := value.(V)
	return assertVal, ok
}

// sync.Map wrapper ----

func (v *Map[K, V]) Load(key K) (V, bool) {
	return v.Assert(v.m.Load(key))
}

func (v *Map[K, V]) Store(key K, value V) {
	v.m.Store(key, value)
}

func (v *Map[K, V]) LoadOrStore(key K, value V) (V, bool) {
	return v.Assert(v.m.LoadOrStore(key, value))
}

func (v *Map[K, V]) LoadAndDelete(key K) (V, bool) {
	return v.Assert(v.m.LoadAndDelete(key))
}

func (v *Map[K, V]) Delete(key K) {
	v.m.Delete(key)
}

func (v *Map[K, V]) Swap(key K, value V) (V, bool) {
	return v.Assert(v.m.Swap(key, value))
}

func (v *Map[K, V]) CompareAndSwap(key K, old, new V) bool {
	return v.m.CompareAndSwap(key, old, new)
}

func (v *Map[K, V]) CompareAndDelete(key K, old V) bool {
	return v.m.CompareAndDelete(key, old)
}

func (v *Map[K, V]) Range(f func(key K, value V) bool) {
	v.m.Range(func(key, value any) bool {
		return f(key.(K), value.(V))
	})
}

func (v *Map[K, V]) Len() int64 {
	var count atomic.Int64
	v.m.Range(func(_, _ any) bool {
		count.Add(1)
		return true
	})
	return count.Load()
}

func New[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{}
}
