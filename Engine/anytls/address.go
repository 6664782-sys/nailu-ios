package anytls

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
)

// UOT payload addresses have different ATYP values from SOCKS5. The UOT v2
// request itself still uses ordinary SOCKS5 address types.
func Address(host string, port uint16, uot bool) ([]byte, error) {
	var out []byte
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			tag := byte(1)
			if uot {
				tag = 0
			}
			out = append([]byte{tag}, ip4...)
		} else {
			tag := byte(4)
			if uot {
				tag = 1
			}
			out = append([]byte{tag}, ip.To16()...)
		}
	} else {
		if len(host) == 0 || len(host) > 255 {
			return nil, errors.New("invalid target host")
		}
		tag := byte(3)
		if uot {
			tag = 2
		}
		out = append([]byte{tag, byte(len(host))}, []byte(host)...)
	}
	return binary.BigEndian.AppendUint16(out, port), nil
}
func ReadAddress(r io.Reader, uot bool) (string, uint16, error) {
	var b [1]byte
	if _, e := io.ReadFull(r, b[:]); e != nil {
		return "", 0, e
	}
	kind := b[0]
	if uot {
		switch kind {
		case 0:
			kind = 1
		case 1:
			kind = 4
		case 2:
			kind = 3
		default:
			return "", 0, errors.New("invalid UOT address type")
		}
	}
	length := 0
	switch kind {
	case 1:
		length = 4
	case 4:
		length = 16
	case 3:
		if _, e := io.ReadFull(r, b[:]); e != nil {
			return "", 0, e
		}
		length = int(b[0])
	default:
		return "", 0, errors.New("invalid address type")
	}
	data := make([]byte, length+2)
	if _, e := io.ReadFull(r, data); e != nil {
		return "", 0, e
	}
	host := string(data[:length])
	if kind != 3 {
		host = net.IP(data[:length]).String()
	}
	return host, binary.BigEndian.Uint16(data[length:]), nil
}
