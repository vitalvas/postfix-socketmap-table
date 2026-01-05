package socketmap

import (
	"bytes"
	"encoding"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetstringInterfaces(t *testing.T) {
	t.Run("implements io.ReaderFrom", func(_ *testing.T) {
		var _ io.ReaderFrom = (*Netstring)(nil)
	})

	t.Run("implements io.WriterTo", func(_ *testing.T) {
		var _ io.WriterTo = (*Netstring)(nil)
	})

	t.Run("implements fmt.Stringer", func(_ *testing.T) {
		var _ fmt.Stringer = (*Netstring)(nil)
	})

	t.Run("implements encoding.BinaryMarshaler", func(_ *testing.T) {
		var _ encoding.BinaryMarshaler = (*Netstring)(nil)
	})

	t.Run("implements encoding.BinaryUnmarshaler", func(_ *testing.T) {
		var _ encoding.BinaryUnmarshaler = (*Netstring)(nil)
	})
}

func TestNetstringReadFrom(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		wantData   []byte
		wantN      int64
		wantErr    error
		wantErrIs  error
		useForRead bool
	}{
		{
			name:       "valid simple netstring",
			input:      []byte("5:hello,"),
			wantData:   []byte("hello"),
			wantN:      8,
			useForRead: true,
		},
		{
			name:       "valid empty netstring",
			input:      []byte("0:,"),
			wantData:   []byte(""),
			wantN:      3,
			useForRead: true,
		},
		{
			name:       "valid long data",
			input:      []byte("10:0123456789,"),
			wantData:   []byte("0123456789"),
			wantN:      14,
			useForRead: true,
		},
		{
			name:       "empty input returns EOF",
			input:      []byte(""),
			wantN:      0,
			wantErr:    io.EOF,
			useForRead: true,
		},
		{
			name:       "incomplete length prefix",
			input:      []byte("5"),
			wantN:      1,
			wantErr:    io.ErrUnexpectedEOF,
			useForRead: true,
		},
		{
			name:       "incomplete data",
			input:      []byte("5:hel"),
			wantN:      5,
			wantErr:    io.ErrUnexpectedEOF,
			useForRead: true,
		},
		{
			name:       "missing trailing comma",
			input:      []byte("5:hello"),
			wantN:      7,
			wantErr:    io.ErrUnexpectedEOF,
			useForRead: true,
		},
		{
			name:       "invalid character in length",
			input:      []byte("5a:hello,"),
			wantN:      2,
			wantErrIs:  ErrNetstringInvalid,
			useForRead: true,
		},
		{
			name:       "colon without length",
			input:      []byte(":hello,"),
			wantN:      1,
			wantErrIs:  ErrNetstringInvalid,
			useForRead: true,
		},
		{
			name:       "wrong trailing character",
			input:      []byte("5:hello;"),
			wantN:      8,
			wantErrIs:  ErrNetstringInvalid,
			useForRead: true,
		},
		{
			name:       "length prefix too long",
			input:      []byte("12345678901:x,"),
			wantN:      11,
			wantErrIs:  ErrNetstringInvalid,
			useForRead: true,
		},
		{
			name:       "not in reading mode",
			input:      []byte("5:hello,"),
			wantN:      0,
			wantErrIs:  ErrNetstringInvalid,
			useForRead: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ns *Netstring
			if tc.useForRead {
				ns = NetstringForReading()
			} else {
				ns = NetstringFrom([]byte("test"))
			}

			n, err := ns.ReadFrom(bytes.NewReader(tc.input))

			assert.Equal(t, tc.wantN, n)

			switch {
			case tc.wantErr != nil:
				assert.Equal(t, tc.wantErr, err)
			case tc.wantErrIs != nil:
				assert.ErrorIs(t, err, tc.wantErrIs)
			default:
				require.NoError(t, err)
				data, err := ns.Bytes()
				require.NoError(t, err)
				assert.Equal(t, tc.wantData, data)
			}
		})
	}
}

func TestNetstringWriteTo(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		ready     bool
		wantBytes []byte
		wantN     int64
		wantErr   error
	}{
		{
			name:      "write simple netstring",
			data:      []byte("hello"),
			ready:     true,
			wantBytes: []byte("5:hello,"),
			wantN:     8,
		},
		{
			name:      "write empty netstring",
			data:      []byte(""),
			ready:     true,
			wantBytes: []byte("0:,"),
			wantN:     3,
		},
		{
			name:    "not ready returns error",
			data:    nil,
			ready:   false,
			wantN:   0,
			wantErr: ErrNetstringNotReady,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ns *Netstring
			if tc.ready {
				ns = NetstringFrom(tc.data)
			} else {
				ns = NetstringForReading()
			}

			var buf bytes.Buffer
			n, err := ns.WriteTo(&buf)

			assert.Equal(t, tc.wantN, n)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantBytes, buf.Bytes())
			}
		})
	}

	t.Run("write error propagates", func(t *testing.T) {
		ns := NetstringFrom([]byte("test"))
		errWriter := &errorWriter{err: errors.New("write failed")}

		n, err := ns.WriteTo(errWriter)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write failed")
		assert.Equal(t, int64(0), n)
	})
}

func TestNetstringString(t *testing.T) {
	tests := []struct {
		name     string
		ns       *Netstring
		expected string
	}{
		{
			name:     "ready netstring",
			ns:       NetstringFrom([]byte("hello world")),
			expected: "hello world",
		},
		{
			name:     "empty data",
			ns:       NetstringFrom([]byte("")),
			expected: "",
		},
		{
			name:     "not ready",
			ns:       NetstringForReading(),
			expected: "<not ready>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.ns.String())
		})
	}
}

func TestNetstringMarshalBinary(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		ready     bool
		wantBytes []byte
		wantErr   error
	}{
		{
			name:      "marshal simple data",
			data:      []byte("hello"),
			ready:     true,
			wantBytes: []byte("5:hello,"),
		},
		{
			name:      "marshal empty data",
			data:      []byte(""),
			ready:     true,
			wantBytes: []byte("0:,"),
		},
		{
			name:    "not ready returns error",
			ready:   false,
			wantErr: ErrNetstringNotReady,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ns *Netstring
			if tc.ready {
				ns = NetstringFrom(tc.data)
			} else {
				ns = NetstringForReading()
			}

			result, err := ns.MarshalBinary()
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantBytes, result)
			}
		})
	}
}

func TestNetstringUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantData []byte
		wantErr  error
	}{
		{
			name:     "unmarshal simple netstring",
			input:    []byte("5:hello,"),
			wantData: []byte("hello"),
		},
		{
			name:     "unmarshal empty netstring",
			input:    []byte("0:,"),
			wantData: []byte(""),
		},
		{
			name:     "unmarshal long data",
			input:    []byte("10:0123456789,"),
			wantData: []byte("0123456789"),
		},
		{
			name:    "too short input",
			input:   []byte("5:"),
			wantErr: ErrNetstringInvalid,
		},
		{
			name:    "empty input",
			input:   []byte(""),
			wantErr: ErrNetstringInvalid,
		},
		{
			name:    "no colon",
			input:   []byte("5hello,"),
			wantErr: ErrNetstringInvalid,
		},
		{
			name:    "invalid character in length",
			input:   []byte("5a:hello,"),
			wantErr: ErrNetstringInvalid,
		},
		{
			name:    "colon without length",
			input:   []byte(":hello,"),
			wantErr: ErrNetstringInvalid,
		},
		{
			name:    "incomplete data",
			input:   []byte("5:hel,"),
			wantErr: ErrNetstringIncomplete,
		},
		{
			name:    "wrong trailing character",
			input:   []byte("5:hello;"),
			wantErr: ErrNetstringInvalid,
		},
		{
			name:    "length prefix too long",
			input:   []byte("12345678901:x,"),
			wantErr: ErrNetstringInvalid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ns := &Netstring{}
			err := ns.UnmarshalBinary(tc.input)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				data, err := ns.Bytes()
				require.NoError(t, err)
				assert.Equal(t, tc.wantData, data)
				assert.True(t, ns.ready)
			}
		})
	}
}

func TestNetstringBytes(t *testing.T) {
	t.Run("ready netstring returns data", func(t *testing.T) {
		ns := NetstringFrom([]byte("test data"))
		data, err := ns.Bytes()
		require.NoError(t, err)
		assert.Equal(t, []byte("test data"), data)
	})

	t.Run("not ready returns error", func(t *testing.T) {
		ns := NetstringForReading()
		data, err := ns.Bytes()
		assert.ErrorIs(t, err, ErrNetstringNotReady)
		assert.Nil(t, data)
	})
}

func TestNetstringMarshal(t *testing.T) {
	t.Run("marshal ready netstring", func(t *testing.T) {
		ns := NetstringFrom([]byte("hello"))
		result, err := ns.Marshal()
		require.NoError(t, err)
		assert.Equal(t, []byte("5:hello,"), result)
	})

	t.Run("not ready returns error", func(t *testing.T) {
		ns := NetstringForReading()
		result, err := ns.Marshal()
		assert.ErrorIs(t, err, ErrNetstringNotReady)
		assert.Nil(t, result)
	})
}

func TestNetstringRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"simple string", []byte("hello world")},
		{"empty string", []byte("")},
		{"binary data", []byte{0x00, 0x01, 0x02, 0xff}},
		{"special chars", []byte("hello\nworld\ttab")},
		{"unicode", []byte("hello")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create and marshal
			ns1 := NetstringFrom(tc.data)
			encoded, err := ns1.MarshalBinary()
			require.NoError(t, err)

			// Unmarshal
			ns2 := &Netstring{}
			err = ns2.UnmarshalBinary(encoded)
			require.NoError(t, err)

			// Verify data matches
			data, err := ns2.Bytes()
			require.NoError(t, err)
			assert.Equal(t, tc.data, data)
		})
	}

	t.Run("ReadFrom WriteTo roundtrip", func(t *testing.T) {
		original := []byte("test data for roundtrip")

		// Create and write
		ns1 := NetstringFrom(original)
		var buf bytes.Buffer
		_, err := ns1.WriteTo(&buf)
		require.NoError(t, err)

		// Read back
		ns2 := NetstringForReading()
		_, err = ns2.ReadFrom(&buf)
		require.NoError(t, err)

		// Verify
		data, err := ns2.Bytes()
		require.NoError(t, err)
		assert.Equal(t, original, data)
	})
}

func TestNetstringReadFromBytesTracking(t *testing.T) {
	t.Run("tracks bytes correctly", func(t *testing.T) {
		input := []byte("5:hello,")
		ns := NetstringForReading()
		n, err := ns.ReadFrom(bytes.NewReader(input))
		require.NoError(t, err)
		assert.Equal(t, int64(len(input)), n)
	})

	t.Run("tracks bytes on error", func(t *testing.T) {
		input := []byte("5:hel")
		ns := NetstringForReading()
		n, err := ns.ReadFrom(bytes.NewReader(input))
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
		assert.Equal(t, int64(5), n)
	})
}

func TestNetstringEOFMidParse(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"EOF during length prefix", []byte("12")},
		{"EOF after colon", []byte("5:")},
		{"EOF during data", []byte("5:hel")},
		{"EOF before trailing comma", []byte("5:hello")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ns := NetstringForReading()
			_, err := ns.ReadFrom(bytes.NewReader(tc.input))
			assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
		})
	}
}

// errorWriter is a helper for testing write errors
type errorWriter struct {
	err error
}

func (w *errorWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

// zeroReader returns (0, nil) a limited number of times, then returns data or EOF
type zeroReader struct {
	zeroCount int
	maxZeros  int
	data      []byte
	pos       int
}

func (r *zeroReader) Read(p []byte) (int, error) {
	if r.zeroCount < r.maxZeros {
		r.zeroCount++
		return 0, nil
	}
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func TestNetstringReadFromZeroReader(t *testing.T) {
	t.Run("handles zero reads before data", func(t *testing.T) {
		// Reader returns (0, nil) 3 times before returning actual data
		r := &zeroReader{
			maxZeros: 3,
			data:     []byte("5:hello,"),
		}

		ns := NetstringForReading()

		// First 3 calls should return ErrNetstringIncomplete
		for i := 0; i < 3; i++ {
			_, err := ns.ReadFrom(r)
			assert.ErrorIs(t, err, ErrNetstringIncomplete)
		}

		// Next call should succeed
		_, err := ns.ReadFrom(r)
		require.NoError(t, err)

		data, err := ns.Bytes()
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), data)
	})

	t.Run("handles zero reads then EOF", func(t *testing.T) {
		// Reader returns (0, nil) 2 times, then EOF
		r := &zeroReader{
			maxZeros: 2,
			data:     []byte{}, // empty, will return EOF after zeros
		}

		ns := NetstringForReading()

		// First 2 calls should return ErrNetstringIncomplete
		for i := 0; i < 2; i++ {
			_, err := ns.ReadFrom(r)
			assert.ErrorIs(t, err, ErrNetstringIncomplete)
		}

		// Next call should return EOF
		_, err := ns.ReadFrom(r)
		assert.ErrorIs(t, err, io.EOF)
	})
}

func BenchmarkNetstringReadFrom(b *testing.B) {
	data := []byte("23:testmap foo@example.com,")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := NetstringForReading()
		_, _ = ns.ReadFrom(bytes.NewReader(data))
	}
}

func BenchmarkNetstringWriteTo(b *testing.B) {
	ns := NetstringFrom([]byte("testmap foo@example.com"))
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_, _ = ns.WriteTo(&buf)
	}
}

func BenchmarkNetstringMarshalBinary(b *testing.B) {
	ns := NetstringFrom([]byte("testmap foo@example.com"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ns.MarshalBinary()
	}
}

func BenchmarkNetstringUnmarshalBinary(b *testing.B) {
	data := []byte("23:testmap foo@example.com,")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := &Netstring{}
		_ = ns.UnmarshalBinary(data)
	}
}

func FuzzNetstringParsing(f *testing.F) {
	f.Add([]byte("5:hello,"))
	f.Add([]byte("0:,"))
	f.Add([]byte("10:0123456789,"))
	f.Add([]byte(""))
	f.Add([]byte("abc"))
	f.Add([]byte("5:hi,"))
	f.Add([]byte("999999:"))

	f.Fuzz(func(_ *testing.T, data []byte) {
		ns := NetstringForReading()
		_, _ = ns.ReadFrom(bytes.NewReader(data))
	})
}
