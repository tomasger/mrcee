package job

import (
	"io"
	"sync"
)

// output retains combined stdout and stderr in an append-only buffer.
// It notifies readers when new data arrives or the producer finishes.
type output struct {
	m sync.Mutex
	// TODO: Support disk-backed output storage for large outputs
	data     []byte
	changed  chan struct{}
	finished bool
}

// outputReader implements io.ReadCloser with a read offset.
// It reads buffered output, then waits for more data until the producer
// finishes, or the reader is closed.
type outputReader struct {
	mu           sync.Mutex
	output       *output
	offset       int
	readerClosed chan struct{}
	closeOnce    sync.Once
}

// Write is an implementation of io.Writer.
// Invoked by exec.Cmd when a process produces new output.
// Writes after finish return io.ErrClosedPipe without modifying the buffer.
func (o *output) Write(b []byte) (int, error) {
	o.m.Lock()
	defer o.m.Unlock()

	if o.finished {
		return 0, io.ErrClosedPipe
	}
	if len(b) == 0 {
		return 0, nil
	}

	o.data = append(o.data, b...)

	// broadcast to changes to every listener
	close(o.changed)
	o.changed = make(chan struct{})

	return len(b), nil
}

// readAt returns output from a selected offset up to the size of len(b)
// or all available output, whichever is smaller.
// `finished` indicates whether the producer (process) has finished writing to `output.data`,
// it is up to the called of `readAt` to call readAt until `length == 0` to ensure
// all output is fetched.
func (o *output) readAt(b []byte, offset int) (length int, finished bool, changed <-chan struct{}) {
	o.m.Lock()
	defer o.m.Unlock()

	length = copy(b, o.data[offset:])
	return length, o.finished, o.changed
}

func (o *output) finish() {
	o.m.Lock()
	defer o.m.Unlock()

	if !o.finished {
		o.finished = true
		close(o.changed)
	}
}

func (o *outputReader) Read(b []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for {
		select {
		case <-o.readerClosed:
			return 0, io.ErrClosedPipe
		default:
		}

		if len(b) == 0 {
			return 0, nil
		}

		length, finished, changed := o.output.readAt(b, o.offset)
		if length > 0 {
			o.offset += length
			return length, nil
		}
		if finished {
			return 0, io.EOF
		}

		select {
		case <-changed: // indicates new data or completion
		case <-o.readerClosed:
			return 0, io.ErrClosedPipe
		}
	}
}

func (o *outputReader) Close() error {
	o.closeOnce.Do(func() {
		close(o.readerClosed)
	})
	return nil
}
