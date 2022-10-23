package socketmap

import "github.com/seandlg/netstring"

type ReplyType string

const (
	ReplyTypeOK       ReplyType = "OK"
	ReplyTypeTEMP     ReplyType = "TEMP"
	ReplyTypeNOTFOUND ReplyType = "NOTFOUND"
	ReplyTypeTIMEOUT  ReplyType = "TIMEOUT"
	ReplyTypePERM     ReplyType = "PERM"
)

// A result sent to Postfix
type Result struct {
	Status ReplyType
	Data   string
}

func (r *Result) Encode() *netstring.Netstring {
	return netstring.From([]byte(string(r.Status) + " " + r.Data))
}

func ReplyOK(data string) *Result {
	return &Result{
		Status: ReplyTypeOK,
		Data:   data,
	}
}

func ReplyNotFound() *Result {
	return &Result{
		Status: ReplyTypeNOTFOUND,
		Data:   "",
	}
}

func ReplyTempFail(reason string) *Result {
	return &Result{
		Status: ReplyTypeTEMP,
		Data:   reason,
	}
}

func ReplyTimeout(reason string) *Result {
	return &Result{
		Status: ReplyTypeTIMEOUT,
		Data:   reason,
	}
}

func ReplyPermFail(reason string) *Result {
	return &Result{
		Status: ReplyTypePERM,
		Data:   reason,
	}
}
