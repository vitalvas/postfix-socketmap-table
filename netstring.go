package socketmap

import (
	"errors"
	"io"
	"strconv"
)

// Netstring errors
var (
	// ErrNetstringIncomplete is returned when the netstring data is incomplete.
	ErrNetstringIncomplete = errors.New("netstring: incomplete data")

	// ErrNetstringInvalid is returned when the netstring format is invalid.
	ErrNetstringInvalid = errors.New("netstring: invalid format")

	// ErrNetstringNotReady is returned when trying to read data before it's ready.
	ErrNetstringNotReady = errors.New("netstring: data not ready")
)

// Compile-time interface checks
var (
	_ io.ReaderFrom = (*Netstring)(nil)
	_ io.WriterTo   = (*Netstring)(nil)
)

// Netstring represents a netstring-encoded message.
// Format: <length>:<data>,
type Netstring struct {
	data     []byte
	ready    bool
	reading  bool
	lenBuf   []byte
	dataLen  int
	dataRead int
}

// NetstringForReading creates a new Netstring ready for reading from a stream.
func NetstringForReading() *Netstring {
	return &Netstring{
		reading: true,
		lenBuf:  make([]byte, 0, 16),
	}
}

// NetstringFrom creates a new Netstring from the given data.
func NetstringFrom(data []byte) *Netstring {
	return &Netstring{
		data:  data,
		ready: true,
	}
}

// ReadFrom reads a netstring from the given reader.
// Implements io.ReaderFrom interface.
// Returns the number of bytes read and nil when complete,
// ErrNetstringIncomplete if more data is needed,
// or another error if the format is invalid or an I/O error occurs.
func (ns *Netstring) ReadFrom(r io.Reader) (int64, error) {
	if !ns.reading {
		return 0, ErrNetstringInvalid
	}

	var totalRead int64
	buf := make([]byte, 1)

	// Read length prefix until we find ':'
	if ns.dataLen == 0 && !ns.ready {
		for {
			n, err := r.Read(buf)
			totalRead += int64(n)
			if err != nil {
				if err == io.EOF && len(ns.lenBuf) == 0 {
					return totalRead, io.EOF
				}
				if err == io.EOF {
					return totalRead, io.ErrUnexpectedEOF
				}
				return totalRead, err
			}
			if n == 0 {
				return totalRead, ErrNetstringIncomplete
			}

			c := buf[0]
			if c == ':' {
				if len(ns.lenBuf) == 0 {
					return totalRead, ErrNetstringInvalid
				}
				length, err := strconv.Atoi(string(ns.lenBuf))
				if err != nil {
					return totalRead, ErrNetstringInvalid
				}
				ns.dataLen = length
				ns.data = make([]byte, length)
				ns.dataRead = 0
				break
			}

			if c < '0' || c > '9' {
				return totalRead, ErrNetstringInvalid
			}

			ns.lenBuf = append(ns.lenBuf, c)
			if len(ns.lenBuf) > 10 {
				return totalRead, ErrNetstringInvalid
			}
		}
	}

	// Read data
	for ns.dataRead < ns.dataLen {
		n, err := r.Read(ns.data[ns.dataRead:])
		totalRead += int64(n)
		if err != nil {
			if err == io.EOF {
				return totalRead, io.ErrUnexpectedEOF
			}
			return totalRead, err
		}
		if n == 0 {
			return totalRead, ErrNetstringIncomplete
		}
		ns.dataRead += n
	}

	// Read trailing comma
	n, err := r.Read(buf)
	totalRead += int64(n)
	if err != nil {
		if err == io.EOF {
			return totalRead, io.ErrUnexpectedEOF
		}
		return totalRead, err
	}
	if n == 0 {
		return totalRead, ErrNetstringIncomplete
	}
	if buf[0] != ',' {
		return totalRead, ErrNetstringInvalid
	}

	ns.ready = true
	return totalRead, nil
}

// WriteTo writes the netstring-encoded data to the given writer.
// Implements io.WriterTo interface.
func (ns *Netstring) WriteTo(w io.Writer) (int64, error) {
	if !ns.ready {
		return 0, ErrNetstringNotReady
	}

	data, err := ns.Marshal()
	if err != nil {
		return 0, err
	}

	n, err := w.Write(data)
	return int64(n), err
}

// String returns a string representation of the netstring data.
// Implements fmt.Stringer interface.
func (ns *Netstring) String() string {
	if !ns.ready {
		return "<not ready>"
	}
	return string(ns.data)
}

// MarshalBinary returns the netstring-encoded representation.
// Implements encoding.BinaryMarshaler interface.
func (ns *Netstring) MarshalBinary() ([]byte, error) {
	return ns.Marshal()
}

// UnmarshalBinary decodes a netstring from the given data.
// Implements encoding.BinaryUnmarshaler interface.
func (ns *Netstring) UnmarshalBinary(data []byte) error {
	if len(data) < 3 {
		return ErrNetstringInvalid
	}

	// Find the colon separator
	colonIdx := -1
	for i := 0; i < len(data) && i < 11; i++ {
		if data[i] == ':' {
			colonIdx = i
			break
		}
		if data[i] < '0' || data[i] > '9' {
			return ErrNetstringInvalid
		}
	}

	if colonIdx == -1 || colonIdx == 0 {
		return ErrNetstringInvalid
	}

	// Parse length
	length, err := strconv.Atoi(string(data[:colonIdx]))
	if err != nil {
		return ErrNetstringInvalid
	}

	// Verify we have enough data: length prefix + ':' + data + ','
	expectedLen := colonIdx + 1 + length + 1
	if len(data) < expectedLen {
		return ErrNetstringIncomplete
	}

	// Verify trailing comma
	if data[colonIdx+1+length] != ',' {
		return ErrNetstringInvalid
	}

	// Extract data
	ns.data = make([]byte, length)
	copy(ns.data, data[colonIdx+1:colonIdx+1+length])
	ns.ready = true
	ns.reading = false

	return nil
}

// Bytes returns the data contained in the netstring.
func (ns *Netstring) Bytes() ([]byte, error) {
	if !ns.ready {
		return nil, ErrNetstringNotReady
	}
	return ns.data, nil
}

// Marshal returns the netstring-encoded representation.
func (ns *Netstring) Marshal() ([]byte, error) {
	if !ns.ready {
		return nil, ErrNetstringNotReady
	}

	length := strconv.Itoa(len(ns.data))
	result := make([]byte, 0, len(length)+1+len(ns.data)+1)
	result = append(result, length...)
	result = append(result, ':')
	result = append(result, ns.data...)
	result = append(result, ',')
	return result, nil
}
