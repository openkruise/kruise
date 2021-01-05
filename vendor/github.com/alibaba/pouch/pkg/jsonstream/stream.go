package jsonstream

import (
	"bytes"
	"io"
	"sync"

	"github.com/sirupsen/logrus"
)

// JSONStream represents a stream transfer, the data be encoded with json.
type JSONStream struct {
	w     io.Writer
	cache chan interface{}
	wg    sync.WaitGroup
	f     Formater
}

// New creates a 'JSONStream' instance and use a goroutine to recv/send data.
func New(out io.Writer, f Formater) *JSONStream {
	format := f
	if format == nil {
		format = newDefaultFormat()
	}

	stream := &JSONStream{
		w:     out,
		cache: make(chan interface{}, 16),
		f:     format,
	}

	stream.wg.Add(1)

	go func() {
		defer stream.wg.Done()

		f := stream.f

		// begin to write.
		if err := stream.writeDelim(f.BeginWrite); err != nil {
			logrus.Errorf("failed to write begin delim: %v", err)
			return
		}

		for {
			o, ok := <-stream.cache
			if !ok && o == nil {
				break
			}

			if err := stream.writeObj(o, f.Write); err != nil {
				logrus.Errorf("failed to write object: %v", err)
				return
			}
		}

		// end to write.
		if err := stream.writeDelim(f.EndWrite); err != nil {
			logrus.Errorf("failed to write end delim: %v", err)
		}
	}()

	return stream
}

// Wait waits the stream to finish.
func (s *JSONStream) Wait() {
	s.wg.Wait()
}

// WriteObject writes a object to client via stream tranfer.
func (s *JSONStream) WriteObject(obj interface{}) error {
	s.cache <- obj
	return nil
}

// Close closes the stream transfer.
func (s *JSONStream) Close() error {
	close(s.cache)
	return nil
}

func (s *JSONStream) writeDelim(f func() ([]byte, error)) error {
	b, err := f()
	if err != nil {
		return err
	}
	return s.write(bytes.NewBuffer(b))
}

func (s *JSONStream) writeObj(o interface{}, f func(interface{}) ([]byte, error)) error {
	b, err := f(o)
	if err != nil {
		return err
	}

	return s.write(bytes.NewBuffer(b))
}

func (s *JSONStream) write(buf *bytes.Buffer) error {
	for {
		if _, err := buf.WriteTo(s.w); err != nil {
			if err == io.ErrShortWrite {
				continue
			}
			return err
		}
		break
	}
	return nil
}
