package anytls

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math/big"
	"strconv"
	"strings"
)

// Protocol layout is implemented from the published AnyTLS specification.
// No upstream reference-client implementation is linked or copied.
const defaultScheme = "stop=8\n0=30-30\n1=100-400\n2=400-500,c,500-1000,c,500-1000,c,500-1000,c,500-1000\n3=9-9,500-1000\n4=500-1000\n5=500-1000\n6=500-1000\n7=500-1000"

type sizeRange struct {
	low, high int
	check     bool
}
type padding struct {
	raw     string
	stop    int
	records map[int][]sizeRange
}

func parsePadding(raw string) (*padding, error) {
	if len(raw) > 32768 {
		return nil, errors.New("padding scheme too large")
	}
	p := &padding{raw: raw, records: map[int][]sizeRange{}}
	for _, line := range strings.Split(raw, "\n") {
		kv := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(kv) != 2 {
			return nil, errors.New("invalid padding scheme")
		}
		if kv[0] == "stop" {
			n, e := strconv.Atoi(kv[1])
			if e != nil || n < 1 || n > 64 {
				return nil, errors.New("invalid padding stop")
			}
			p.stop = n
			continue
		}
		index, e := strconv.Atoi(kv[0])
		if e != nil || index < 0 || index > 63 {
			return nil, errors.New("invalid padding index")
		}
		tokens := strings.Split(kv[1], ",")
		if len(tokens) > 32 {
			return nil, errors.New("too many padding segments")
		}
		for _, token := range tokens {
			if token == "c" {
				p.records[index] = append(p.records[index], sizeRange{check: true})
				continue
			}
			bounds := strings.Split(token, "-")
			if len(bounds) != 2 {
				return nil, errors.New("invalid padding range")
			}
			low, e := strconv.Atoi(bounds[0])
			high, e2 := strconv.Atoi(bounds[1])
			if e != nil || e2 != nil || low < 0 || high < low || high > 8192 {
				return nil, errors.New("padding range exceeds memory limit")
			}
			p.records[index] = append(p.records[index], sizeRange{low: low, high: high})
		}
	}
	if p.stop == 0 {
		return nil, errors.New("missing padding stop")
	}
	return p, nil
}
func (p *padding) digest() string { d := md5.Sum([]byte(p.raw)); return hex.EncodeToString(d[:]) }
func choose(r sizeRange) int {
	if r.high == r.low {
		return r.low
	}
	n, e := rand.Int(rand.Reader, big.NewInt(int64(r.high-r.low+1)))
	if e != nil {
		return r.low
	}
	return r.low + int(n.Int64())
}
