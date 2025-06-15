package bundle

import "time"

func Bundle[T any](out, in chan T, delay time.Duration) {
	type bundle struct {
		id    uint64
		value T
	}
	go func() {
		ch := make(chan bundle, 1024)
		var lastId uint64
		for {
			select {
			case t := <-ch:
				if t.id == lastId {
					out <- t.value
				}
			case v, ok := <-in:
				if !ok {
					return
				}
				lastId++
				go func(lastId uint64) {
					time.Sleep(delay)
					ch <- bundle{lastId, v}
				}(lastId)
			}
		}
	}()
}
