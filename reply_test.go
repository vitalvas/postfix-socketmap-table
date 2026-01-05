package socketmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResult(t *testing.T) {
	t.Run("encode", func(t *testing.T) {
		r := &Result{
			Status: ReplyTypeOK,
			Data:   "bar@example.com",
		}

		expected := []byte("18:OK bar@example.com,")
		result, err := r.Encode().Marshal()
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("reply constructors", func(t *testing.T) {
		tests := []struct {
			name     string
			result   *Result
			expected []byte
		}{
			{
				name:     "ReplyOK",
				result:   ReplyOK("value"),
				expected: []byte("8:OK value,"),
			},
			{
				name:     "ReplyNotFound",
				result:   ReplyNotFound(),
				expected: []byte("9:NOTFOUND ,"),
			},
			{
				name:     "ReplyTempFail",
				result:   ReplyTempFail("temporary error"),
				expected: []byte("20:TEMP temporary error,"),
			},
			{
				name:     "ReplyTimeout",
				result:   ReplyTimeout("operation timed out"),
				expected: []byte("27:TIMEOUT operation timed out,"),
			},
			{
				name:     "ReplyPermFail",
				result:   ReplyPermFail("permanent error"),
				expected: []byte("20:PERM permanent error,"),
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				result, err := tc.result.Encode().Marshal()
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			})
		}
	})
}

func FuzzResultEncode(f *testing.F) {
	f.Add("OK", "value")
	f.Add("NOTFOUND", "")
	f.Add("TEMP", "error message")
	f.Add("TIMEOUT", "timed out")
	f.Add("PERM", "permanent error")
	f.Add("", "")
	f.Add("STATUS", "data with special chars: \n\t\r")

	f.Fuzz(func(_ *testing.T, status, data string) {
		r := &Result{
			Status: ReplyType(status),
			Data:   data,
		}
		_, _ = r.Encode().Marshal()
	})
}
