// +build linux

package gnotify

import (
	"errors"
	"github/lightmen/gnotify/netpoll"
	"golang.org/x/sys/unix"
	"io"
	"path"
	"sync"
	"unsafe"
)

type Watcher struct {
	Event  chan Event
	Err    chan error
	poller *netpoll.Poller
	fd     int        //File descriptor (as returned by the inotify_init() syscall)
	mu     sync.Mutex //map lock
	watchs map[string]*Watch
	paths  map[int]string
	buf    []byte
}

type Watch struct {
	fd   int
	Mask uint32
}

var (
	Mask2Op = map[uint32]Op{
		unix.IN_CREATE:      Create,
		unix.IN_DELETE_SELF: Delete,
		unix.IN_MOVE_SELF:   Delete,
		unix.IN_MODIFY:      Modify,
	}
)

func NewWatcher() (watcher *Watcher, err error) {
	var (
		poller *netpoll.Poller
		fd     int
	)

	opts := []netpoll.OptionFunc{netpoll.WithEventSize(7)}
	if poller, err = netpoll.New(opts...); err != nil {
		return
	}

	if fd, err = unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK); err != nil {
		return
	}

	if err = poller.AddRead(fd); err != nil {
		goto err1
	}

	watcher = &Watcher{
		fd:     fd,
		poller: poller,
		Event:  make(chan Event),
		Err:    make(chan error),
		watchs: make(map[string]*Watch),
		paths:  make(map[int]string),
	}

	go watcher.waitEvents()
	return

err1:
	unix.Close(fd)
	return
}

func (w *Watcher) waitEvents() {
	defer w.Close()

	w.buf = make([]byte, unix.SizeofInotifyEvent*4096)
	for {
		events, err := w.poller.Wait()
		if err != nil {
			w.Err <- err
			return
		}

		isWakeUp := false
		for _, event := range events {
			if w.poller.IsWake(event) {
				isWakeUp = true
				continue
			}

			err = w.handleEpollEvent(event)
			if err != nil && err != unix.EINTR {
				w.Err <- err
				return
			}
		}

		if isWakeUp {
			return
		}
	}
}

func (w *Watcher) Close() {
	w.poller.Close()
	unix.Close(w.fd)
	close(w.Event)
	close(w.Err)
}

func (w *Watcher) handleEpollEvent(epEvent unix.EpollEvent) (err error) {
	if w.fd != int(epEvent.Fd) { //仅处理inotify的文件事件
		return
	}

	buf := w.buf[:]
	n, err := unix.Read(w.fd, buf)
	if err != nil {
		if err == unix.EINTR {
			err = nil
		}
		return
	}

	if n < unix.SizeofInotifyEvent {
		if n == 0 {
			return io.EOF
		}
		return errors.New("notify: short read in handleEvent")
	}

	var offset uint32
	for offset <= uint32(n-unix.SizeofInotifyEvent) {
		ievent := (*unix.InotifyEvent)(unsafe.Pointer(&buf[offset]))

		err = w.handleInotifyEvent(ievent)
		if err != nil {
			return
		}

		offset += (unix.SizeofInotifyEvent + ievent.Len)
	}

	return
}

func (w *Watcher) handleInotifyEvent(ievent *unix.InotifyEvent) (err error) {
	wfd := int(ievent.Wd)
	mask := ievent.Mask

	w.mu.Lock()
	fileName, ok := w.paths[wfd]
	if !ok {
		w.mu.Unlock()
		return
	}

	op, ok := w.GetOp(mask)
	if !ok {
		return
	}

	watch := w.watchs[fileName]
	if op&Delete != 0 {
		delete(w.watchs, fileName)
		delete(w.paths, wfd)
	}
	w.mu.Unlock()

	if watch.Mask&mask == 0 {
		return
	}

	event := Event{
		Name: fileName,
		Op:   op,
	}
	w.Event <- event
	return
}

func (w *Watcher) GetOp(mask uint32) (op Op, ok bool) {
	for k, v := range Mask2Op {
		if mask&k == 0 {
			continue
		}
		op = v
		ok = true
		break
	}

	return
}

func (w *Watcher) GetMask(op Op) (mask uint32) {
	for k, v := range Mask2Op {
		if op&v == 0 {
			continue
		}

		mask |= k
	}

	return
}

func (w *Watcher) Add(fileName string, op Op) (err error) {
	if op == 0 {
		op = AllOp
	}

	fileName = path.Clean(fileName)

	mask := w.GetMask(op)
	w.mu.Lock()
	defer w.mu.Unlock()

	watch, ok := w.watchs[fileName]
	if ok {
		watch.Mask = mask
		return
	}

	wfd, err := unix.InotifyAddWatch(w.fd, fileName, mask)
	if err != nil {
		return
	}

	watch = &Watch{
		fd:   wfd,
		Mask: mask,
	}

	w.watchs[fileName] = watch
	w.paths[wfd] = fileName

	return

}
