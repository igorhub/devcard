package project

import "time"

func bundle(dur time.Duration, src <-chan struct{}, dst chan<- struct{}) {
	ch := make(chan uint64)
	var currentId uint64
	go func() {
		for {
			select {
			case _, ok := <-src:
				if !ok {
					return
				}
				currentId++
				go func(id uint64) {
					time.Sleep(dur)
					ch <- id
				}(currentId)
			case id := <-ch:
				if id == currentId {
					dst <- struct{}{}
				}
			}
		}
	}()
}
