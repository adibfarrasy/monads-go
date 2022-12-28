package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

/* Functors
a mapping between categories. can be thought of as a 'container', e.g.
- slice: `[]T` is a container where the items are ordered into a list.
- channel: `<-chan T` is a container that passes items of type T.
- pointer: `*T` is a container that may be empty of contain one item.
- function: `func(A) T` is a container, like a lock box that first needs a key A before you can see the item T.
- multiple return values: `func() (T, error)` is a container that possibly contains one item.

functor requires `fmap` function for the 'container' type.
```
func fmap[A, B any](f func(A)) B, aContainerOfA Container[A]) Container[B]
```
*/

func fmapSlice[A, B any](f func(A) B, as []A) []B {
	bs := make([]B, len(as))
	for i, a := range as {
		bs[i] = f(a)
	}
	return bs
}

func fmapChan[A, B any](f func(A) B, in <-chan A) <-chan B {
	out := make(chan B, cap(in))
	go func() {
		for a := range in {
			b := f(a)
			out <- b
		}
		close(out)
	}()
	return out
}

func fmapPointer[A, B any](f func(A) B, a *A) *B {
	if a == nil {
		return nil
	}
	b := f(*a)
	return &b
}

func fmapFunc[A, B, C any](f func(B) C, g func(A) B) func(A) C {
	return func(a A) C {
		b := g(a)
		return f(b)
	}
}

func fmapFuncErr[A, B any](f func(A) B, g func() (*A, error)) func() (*B, error) {
	return func() (*B, error) {
		a, err := g()
		if err != nil {
			return nil, err
		}
		b := f(*a)
		return &b, nil
	}
}

/* Join
To compose functions that return a monad (an 'embellished type'), the functor (with `fmap` property) needs another property `join` with signature
```
type join[C any] func(M[M[C]]) M[C]
```
*/

// Monadic slice
func joinSlice[T any](ss [][]T) []T {
	s := []T{}
	for i := range ss {
		s = append(s, ss[i]...)
	}
	return s
}

func trueFmapSlice[A, B any](f func(A) []B, as []A) [][]B {
	bss := make([][]B, len(as))
	for i, a := range as {
		bss[i] = f(a)
	}
	return bss
}

func composeSlice[A, B, C any](f func(A) []B, g func(B) []C) func(A) []C {
	return func(a A) []C {
		bs := f(a)
		css := trueFmapSlice(g, bs)
		cs := joinSlice(css)
		return cs
	}
}

// Monadic error handling
func joinFuncErr[C any](f func() (C, error), err error) (C, error) {
	if err != nil {
		return *new(C), err
	}
	return f()
}

func trueFmapFuncErr[A, B, C any](
	f func(B) (C, error),
	g func(A) (B, error),
) func(A) (func() (C, error), error) {
	return func(a A) (func() (C, error), error) {
		b, err := g(a)
		if err != nil {
			return nil, err
		}
		c, err := f(b)
		return func() (C, error) {
			return c, err
		}, nil
	}
}

func composeFuncErr[A, B, C any](f func(A) (B, error), g func(B) (C, error)) func(A) (C, error) {
	return func(a A) (C, error) {
		cerr, err := trueFmapFuncErr(g, f)(a)
		c, err := joinFuncErr(cerr, err)
		return c, err
	}
}

// Monadic channel
func joinChan[T any](in <-chan <-chan T) <-chan T {
	out := make(chan T)
	go func() {
		wait := sync.WaitGroup{}
		for c := range in {
			wait.Add(1)
			go func(inner <-chan T) {
				for t := range inner {
					out <- t
				}
				wait.Done()
			}(c)
		}
		wait.Wait()
		close(out)
	}()
	return out
}

func composeChan[A, B, C any](f func(A) <-chan B, g func(B) <-chan C) func(A) <-chan C {
	return func(a A) <-chan C {
		chanOfB := f(a)
		return joinChan(fmapChan(g, chanOfB))
	}
}

// Monadic channel Example
func toChan(lines []string) <-chan string {
	c := make(chan string)
	go func() {
		for _, line := range lines {
			c <- line
		}
		close(c)
	}()
	return c
}

func wordSize(line string) <-chan int {
	c := make(chan int)
	go func() {
		words := strings.Split(line, " ")
		for _, word := range words {
			c <- len(word)
		}
		close(c)
	}()
	return c
}

func monadicChanTest() {
	sizes := composeChan(
		toChan,
		wordSize,
	)([]string{
		"Bart: Eat my monads!",
		"Monads: I don't think that's a very good idea.",
		"what the fuckle is going on",
	})
	total := 0
	for size := range sizes {
		if size > 3 {
			total += 1
		}
	}
	fmt.Println(total)
}

func main() {
	// testFunctors()
	monadicSliceTest()
	monadicErrorHandlingTest()
	monadicChanTest()
}

func testFunctors() {
	aslice := []int{1, 2, 3}
	bslice := fmapSlice(func(a int) int { return a + 1 }, aslice)
	fmt.Println(bslice)

	achan := make(chan int)
	var wg sync.WaitGroup
	go populate(achan)
	bchan := fmapChan(func(a int) int { return a + 1 }, achan)
	go consume(&wg, bchan)
	time.Sleep(time.Duration(1) * time.Millisecond)

	aptr := &dummy{value: 1}
	fmt.Println(aptr)
	bptr := fmapPointer(func(a dummy) string { return fmt.Sprint(a.value) }, aptr)
	fmt.Println(bptr)

	afn := func(c int) int { return c + 1 }
	bfn := fmapFunc(func(a int) bool { return a > 0 }, afn)
	fmt.Println(bfn(69))

	afnerr := func() (*int, error) { result := 1; return &result, nil }
	bfnerr := fmapFuncErr(func(a int) bool { return a > 0 }, afnerr)
	fmt.Println(bfnerr())
}

type dummy struct {
	value int
}

func populate(ch chan int) {
	for i := 1; i <= 3; i++ {
		ch <- i
	}
}

func consume(wg *sync.WaitGroup, ch <-chan int) {
	for s := range ch {
		fmt.Println(s)
	}
}

// Monadic Slice Example
func upto(n int) []int64 {
	nums := make([]int64, n)
	for i := range nums {
		nums[i] = int64(i + 1)
	}
	return nums
}

func pair(x int64) []string {
	return []string{strconv.FormatInt(x, 10), strconv.FormatInt(-1*x, 10)}
}

func monadicSliceTest() { fmt.Println(composeSlice(upto, pair)(3)) }

// Monadic Error Handling Example
func unmarshal(data []byte) (s string, err error) {
	err = json.Unmarshal(data, &s)
	return
}

func monadicErrorHandlingTest() {
	getnum := composeFuncErr(unmarshal, strconv.Atoi)
	fmt.Println(getnum([]byte("\"1\"")))
}
