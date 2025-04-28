package result

import (
	jsoniter "github.com/json-iterator/go"
	"net/http"
	"sync"
)

var (
	anyPool = sync.Pool{New: func() any {
		return &Wrapper[any]{}
	}}
)

type Result[T any] struct {
	Status  bool   `json:"status"`
	Message string `json:"message,omitempty"`
	Data    T      `json:"data,omitempty"`
}

type Wrapper[T any] struct {
	code   int
	result Result[T]
}

func (t *Wrapper[T]) reset() {
	t.code = 0
	t.result.Status = false
	t.result.Message = ""
	var emptyValue T
	t.result.Data = emptyValue
	return
}

func (t *Wrapper[T]) write(status bool, w http.ResponseWriter) {
	t.result.Status = status

	//	write header
	w.WriteHeader(t.code)
	w.Header().Set("Content-Type", "application/json")

	// encode stream
	_ = jsoniter.ConfigFastest.NewEncoder(w).Encode(t.result)

	// release
	t.reset()
	anyPool.Put(t)

}

func (t *Wrapper[T]) Message(message string) *Wrapper[T] {
	t.result.Message = message
	return t
}

func (t *Wrapper[T]) Data(data T) *Wrapper[T] {
	t.result.Data = data
	return t
}

func (t *Wrapper[T]) Ok(w http.ResponseWriter) {
	t.write(true, w)
}

func (t *Wrapper[T]) Err(w http.ResponseWriter) {
	t.write(false, w)
}

func New[T any](code int) *Wrapper[T] {
	return &Wrapper[T]{
		code:   code,
		result: Result[T]{},
	}
}

func Any(code int) *Wrapper[any] {
	r := anyPool.Get().(*Wrapper[any])
	r.result.Data = nil
	r.code = code
	return r
}
