package harness

// Result is a discriminated union representing a fallible operation.
// Expected failures are returned as Err != nil instead of panicking.
type Result[T any] struct {
	Value T
	Err   error
	OK    bool
}

// OkResult creates a successful Result.
func OkResult[T any](value T) Result[T] {
	return Result[T]{Value: value, OK: true}
}

// ErrResult creates a failed Result.
func ErrResult[T any](err error) Result[T] {
	return Result[T]{Err: err, OK: false}
}

// GetOrThrow returns the success value or panics with the error.
func GetOrThrow[T any](r Result[T]) T {
	if !r.OK {
		panic(r.Err)
	}
	return r.Value
}

// GetOrZero returns the success value or the zero value of T.
func GetOrZero[T any](r Result[T]) T {
	if !r.OK {
		var zero T
		return zero
	}
	return r.Value
}
