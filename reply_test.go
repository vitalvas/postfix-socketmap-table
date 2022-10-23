package socketmap

import "testing"

func TestResultEncode(t *testing.T) {
	r := &Result{
		Status: "OK",
		Data:   "bar@example.com",
	}

	be := []byte("18:OK bar@example.com,")
	bg, err := r.Encode().Marshal()
	if err != nil {
		t.Errorf("Failed to encode: %s", err)
	}
	if string(bg) != string(be) {
		t.Errorf("Encoding yielded invalid netstring data")
	}
}
