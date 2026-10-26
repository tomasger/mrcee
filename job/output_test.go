package job

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestOutput(t *testing.T) (*output, *Registry) {
	t.Helper()
	o := &output{changed: make(chan struct{})}
	r := NewRegistry()
	r.jobs["job"] = &jobEntry{output: o}
	return o, r
}

func openTestStream(t *testing.T, r *Registry) io.ReadCloser {
	t.Helper()
	reader, err := r.OpenStream("job")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, reader.Close()) })
	return reader
}

// TestOutputReplay tests simple happy case for
// ASCII and binary outputs.
func TestOutputReplay(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "text", data: []byte("hello world\n")},
		{name: "binary", data: []byte{0x00, 0x01, 0x7f, 0x80, 0xff}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, r := newTestOutput(t)
			n, err := o.Write(tc.data)
			require.NoError(t, err)
			require.Equal(t, len(tc.data), n)
			o.finish()

			reader := openTestStream(t, r)
			require.Equal(t, tc.data, readAll(t, reader))
		})
	}
}

// TestOutputIndependentOffsets ensures each reader has its own offset.
func TestOutputIndependentOffsets(t *testing.T) {
	o, r := newTestOutput(t)
	want := []byte("hello")
	n, err := o.Write(want)
	require.NoError(t, err)
	require.Equal(t, len(want), n)
	o.finish()

	first := openTestStream(t, r)
	second := openTestStream(t, r)

	// check that partially read offset (first)
	// does not affect another stream (second)
	buf := make([]byte, 2)
	n, err = first.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, want[:2], buf)
	require.Equal(t, want, readAll(t, second))
	require.Equal(t, want[2:], readAll(t, first))

	n, err = first.Read(buf)
	require.Equal(t, 0, n)
	require.ErrorIs(t, err, io.EOF)
}

func TestOutputWriteAfterFinish(t *testing.T) {
	o, r := newTestOutput(t)
	want := []byte("before finish")
	_, err := o.Write(want)
	require.NoError(t, err)
	o.finish()

	n, err := o.Write([]byte("after finish"))
	require.Equal(t, 0, n)
	require.ErrorIs(t, err, io.ErrClosedPipe)

	reader := openTestStream(t, r)
	require.Equal(t, want, readAll(t, reader))
}

// TestOutputReadWaitsForNotification tests that calling `Read()`
// is a blocking operation, and that it unblocks when:
// * write is called
// * job is marked as finished
// * user closes the reader
func TestOutputReadWaitsForNotification(t *testing.T) {
	for _, event := range []string{"write", "finish", "close"} {
		t.Run(event, func(t *testing.T) {
			o, r := newTestOutput(t)
			reader := openTestStream(t, r)
			type result struct {
				n   int
				err error
			}
			done := make(chan result, 1)
			go func() {
				n, err := reader.Read(make([]byte, 1))
				done <- result{n, err}
			}()
			select {
			case got := <-done:
				require.FailNow(t, "Read returned before notification", "%+v", got)
			case <-time.After(20 * time.Millisecond):
			}

			var want result
			switch event {
			case "write":
				_, err := o.Write([]byte{'a'})
				require.NoError(t, err)
				want.n = 1
			case "finish":
				o.finish()
				want.err = io.EOF
			case "close":
				require.NoError(t, reader.Close())
				want.err = io.ErrClosedPipe
			}
			select {
			case got := <-done:
				require.Equal(t, want.n, got.n)
				require.ErrorIs(t, got.err, want.err)
			case <-time.After(5 * time.Second):
				require.FailNow(t, "Read did not wake up")
			}
		})
	}
}

// TestOutputReaderClose ensures idempotency of outputReader
// Close() method, and that it does not affect the output data
// nor other output readers.
func TestOutputReaderClose(t *testing.T) {
	o, r := newTestOutput(t)
	first := openTestStream(t, r)
	second := openTestStream(t, r)
	var wg sync.WaitGroup
	// call Close() multiple times simultaneously
	for range 4 {
		wg.Go(func() {
			assert.NoError(t, first.Close())
		})
	}
	wg.Wait()

	_, err := o.Write([]byte("still running"))
	require.NoError(t, err)
	o.finish()
	// check that closed reader does not receive more data
	n, err := first.Read(make([]byte, 1))
	require.Equal(t, 0, n)
	require.ErrorIs(t, err, io.ErrClosedPipe)
	// ensure other reader is unaffected
	require.Equal(t, "still running", string(readAll(t, second)))
}

// TestOutputZeroLengthRead ensures reader is not blocking when
// a zero length byte slice is passed.
// While it may seem like a user error to pass a zero length slice,
// io.Reader contract explicitly allows that, so we adhere to it.
func TestOutputZeroLengthRead(t *testing.T) {
	_, r := newTestOutput(t)
	reader := openTestStream(t, r)
	done := make(chan struct{})
	go func() {
		defer close(done)
		n, err := reader.Read(make([]byte, 0))
		assert.NoError(t, err)
		assert.Equal(t, 0, n)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "zero-length Read blocked")
	}
}

// TestOutputConcurrentSubscribers checks happy case scenario
// with multiple concurrent subscribers.
func TestOutputConcurrentSubscribers(t *testing.T) {
	o, r := newTestOutput(t)
	dataChunk := []byte("chunk\n")
	want := bytes.Repeat(dataChunk, 100)
	type result struct {
		data []byte
		err  error
	}
	results := make(chan result, 8)
	for range cap(results) {
		reader := openTestStream(t, r)
		go func() {
			data, err := io.ReadAll(reader)
			results <- result{data, err}
		}()
	}
	for i := 0; i < len(want); i += len(dataChunk) {
		_, err := o.Write(want[i : i+len(dataChunk)])
		require.NoError(t, err)
	}
	o.finish()
	for range cap(results) {
		select {
		case got := <-results:
			require.NoError(t, got.err)
			require.Equal(t, want, got.data)
		case <-time.After(5 * time.Second):
			require.FailNow(t, "subscriber did not finish")
		}
	}
}

// readAll reads all available data from a reader
// in a separate goroutine. If reader blocks for a
// prolonged time, it fails the test.
func readAll(t *testing.T, reader io.Reader) []byte {
	t.Helper()
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(reader)
		ch <- result{data, err}
	}()
	select {
	case got := <-ch:
		require.NoError(t, got.err)
		return got.data
	case <-time.After(5 * time.Second):
		require.FailNow(t, "ReadAll did not finish")
		return nil
	}
}
