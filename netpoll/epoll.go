// +build linux

package netpoll

import "golang.org/x/sys/unix"

const (
	readEvents      = unix.EPOLLIN | unix.EPOLLPRI
	writeEvents     = unix.EPOLLOUT
	readWriteEvents = readEvents | writeEvents

	initEventCount = 512

	wakeUpFlag = 0x1
)

type OptionFunc func(opts *Options)

type Options struct {
	EventSize int
}

func WithEventSize(size int) OptionFunc {
	return func(opts *Options) {
		opts.EventSize = size
	}
}

type Poller struct {
	epfd      int //Epoll file descriptor
	wfd       int
	wfdBuf    []byte
	eventList []unix.EpollEvent
	opts      *Options
}

func New(optFuncs ...OptionFunc) (poller *Poller, err error) {
	var (
		epfd int
		wfd  int
	)

	epfd, err = unix.EpollCreate(1024)
	if err != nil {
		return
	}

	opts := &Options{EventSize: initEventCount}
	for _, optFunc := range optFuncs {
		optFunc(opts)
	}

	poller = &Poller{
		epfd: epfd,
		opts: opts,
	}

	wfd, err = unix.Eventfd(0, unix.EFD_NONBLOCK|unix.EFD_CLOEXEC)
	if err != nil {
		goto err1
	}

	if err = poller.AddRead(wfd); err != nil {
		goto err2
	}

	poller.wfd = wfd
	poller.wfdBuf = make([]byte, 8)
	poller.eventList = newEventList(poller.opts.EventSize)
	return

err2:
	unix.Close(wfd)
err1:
	unix.Close(epfd)
	return
}

func newEventList(size int) []unix.EpollEvent {
	eventList := make([]unix.EpollEvent, size)
	return eventList
}

func (p *Poller) AddRead(fd int) (err error) {
	return unix.EpollCtl(p.epfd, unix.EPOLL_CTL_ADD, fd, &unix.EpollEvent{Events: readEvents, Fd: int32(fd)})
}

func (p *Poller) AddWrite(fd int) (err error) {
	return unix.EpollCtl(p.epfd, unix.EPOLL_CTL_ADD, fd, &unix.EpollEvent{Events: writeEvents, Fd: int32(fd)})
}

func (p *Poller) AddReadWrite(fd int) (err error) {
	return unix.EpollCtl(p.epfd, unix.EPOLL_CTL_ADD, fd, &unix.EpollEvent{Events: readWriteEvents, Fd: int32(fd)})
}

func (p *Poller) Wait() (events []unix.EpollEvent, err error) {
	var n int
	for {
		eventList := p.eventList[:]
		n, err = unix.EpollWait(p.epfd, eventList, -1)
		if err != nil && err == unix.EINTR {
			continue
		}

		if err != nil {
			return
		}

		if n <= 0 {
			continue
		}

		events = eventList[:n]
		return
	}
}

func (p *Poller) Wake() (err error) {
	_, err = unix.Write(p.wfd, []byte{wakeUpFlag})
	return
}

func (p *Poller) ClearWake() (err error) {
	buf := make([]byte, 16)
	_, err = unix.Read(p.wfd, buf)
	if err != nil {
		if err == unix.EAGAIN {
			err = nil
		}
		return
	}
	return
}

func (p *Poller) IsWake(event unix.EpollEvent) bool {
	return event.Fd == int32(p.wfd)
}

func (p *Poller) Close() {
	unix.Close(p.epfd)
	unix.Close(p.wfd)
}
