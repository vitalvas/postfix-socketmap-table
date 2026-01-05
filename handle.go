package socketmap

import (
	"context"
	"io"
	"net"
	"time"
)

func (sm *Server) handle(ctx context.Context, conn net.Conn) error {
	defer conn.Close()

	for {
		if sm.config.IdleTimeout > 0 {
			if err := conn.SetReadDeadline(time.Now().Add(sm.config.IdleTimeout)); err != nil {
				return err
			}
		}

		ns, err := sm.netstringRead(conn)
		if err != nil {
			return err
		}

		r := &Request{}
		if err := r.Decode(ns); err != nil {
			return err
		}

		sm.mapsLock.RLock()
		fn, exists := sm.maps[r.Name]
		defaultMap := sm.defaultMap
		sm.mapsLock.RUnlock()

		var res *Result

		switch {
		case exists:
			res, err = fn(ctx, r.Key)
			if err != nil {
				res = ReplyTempFail(err.Error())
			}

		case defaultMap != nil:
			res, err = defaultMap(ctx, r.Name, r.Key)
			if err != nil {
				res = ReplyTempFail(err.Error())
			}

		default:
			res = ReplyTempFail("the lookup map does not exist")
		}

		b, err := res.Encode().Marshal()
		if err != nil {
			return err
		}

		if sm.config.MaxReplySize > 0 && len(b) > sm.config.MaxReplySize {
			return ErrReplyTooLarge
		}

		if sm.config.WriteTimeout > 0 {
			if err := conn.SetWriteDeadline(time.Now().Add(sm.config.WriteTimeout)); err != nil {
				return err
			}
		}

		if _, err := conn.Write(b); err != nil {
			return err
		}
	}
}

func (sm *Server) netstringRead(conn net.Conn) (*Netstring, error) {
	ns := NetstringForReading()

	if sm.config.ReadTimeout > 0 {
		if err := conn.SetReadDeadline(time.Now().Add(sm.config.ReadTimeout)); err != nil {
			return nil, err
		}
	}

	for {
		_, err := ns.ReadFrom(conn)
		if err == nil {
			return ns, nil
		}

		if err == ErrNetstringIncomplete {
			continue
		}

		if err == io.EOF {
			return nil, err
		}

		return nil, err
	}
}
