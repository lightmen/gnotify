package main

import (
	"github/lightmen/gnotify"
	"log"
	"os"
)

func main() {
	watcher, err := gnotify.NewWatcher()
	if err != nil {
		panic(err)
	}

	defer watcher.Close()

	ch := make(chan struct{})

	go func() {
		for {
			select {
			case event, ok := <-watcher.Event:
				if !ok {
					log.Println(" event chan for ok: ", ok)
					return
				}
				log.Printf(" event: %+v", event)
				break
			case err = <-watcher.Err:
				log.Printf("watcher error: %v", err)
			}
		}
	}()

	if len(os.Args) < 1 {
		return
	}

	for i := 1; i < len(os.Args); i++ {
		err = watcher.Add(os.Args[i], 0)
		if err != nil {
			panic(err)
		}
	}
	ch <- struct{}{}
}
