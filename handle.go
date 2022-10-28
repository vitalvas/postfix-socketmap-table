package socketmap

import (
	"context"
	"net"

	"github.com/seandlg/netstring"
)

func (sm *Server) handle(ctx context.Context, conn net.Conn) error {
	defer conn.Close()

	for {
		ns := netstring.ForReading()

		for {
			err := ns.ReadFrom(conn)
			if err == nil {
				break
			}
			if err == netstring.Incomplete {
				continue
			}

			return err
		}

		r := &Request{}
		if err := r.Decode(ns); err != nil {
			return err
		}

		sm.mapsLock.RLock()
		fn, exists := sm.maps[r.Name]
		sm.mapsLock.RUnlock()

		var res *Result
		var err error

		if exists {
			res, err = fn(ctx, r.Key)
			if err != nil {
				res = ReplyTempFail(err.Error())
			}

		} else if sm.defaultMap != nil {
			res, err = sm.defaultMap(ctx, r.Name, r.Key)
			if err != nil {
				res = ReplyTempFail(err.Error())
			}

		} else {
			res = ReplyTempFail("the lookup map does not exist")
		}

		b, err := res.Encode().Marshal()
		if err != nil {
			return err
		}

		if _, err := conn.Write(b); err != nil {
			return err
		}
	}
}
