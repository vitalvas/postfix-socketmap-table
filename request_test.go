package socketmap

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest(t *testing.T) {
	t.Run("decode valid request", func(t *testing.T) {
		b := []byte("23:testmap foo@example.com,")
		ns := NetstringForReading()
		_, err := ns.ReadFrom(bytes.NewReader(b))
		require.NoError(t, err)

		r := &Request{}
		err = r.Decode(ns)
		require.NoError(t, err)
		assert.Equal(t, "testmap", r.Name)
		assert.Equal(t, "foo@example.com", r.Key)
	})

	t.Run("decode malformed request", func(t *testing.T) {
		b := []byte("7:testmap,")
		ns := NetstringForReading()
		_, err := ns.ReadFrom(bytes.NewReader(b))
		require.NoError(t, err)

		r := &Request{}
		err = r.Decode(ns)
		assert.ErrorIs(t, err, errorMalformedRequest)
	})

	t.Run("decode unread netstring returns error", func(t *testing.T) {
		ns := NetstringForReading()

		r := &Request{}
		err := r.Decode(ns)
		assert.Error(t, err)
	})
}

func FuzzRequestDecode(f *testing.F) {
	f.Add([]byte("testmap foo@example.com"))
	f.Add([]byte("map key"))
	f.Add([]byte("a b"))
	f.Add([]byte(""))
	f.Add([]byte("nospace"))
	f.Add([]byte("multiple spaces in key"))
	f.Add([]byte("map "))
	f.Add([]byte(" key"))

	f.Fuzz(func(_ *testing.T, data []byte) {
		ns := NetstringFrom(data)
		r := &Request{}
		_ = r.Decode(ns)
	})
}
