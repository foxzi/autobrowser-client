package fingerprint

import (
	"math/big"
	"math/bits"

	crand "crypto/rand"
)

// mt is the Mersenne Twister of Python's random.Random, seeded with an int,
// so a seed gives the same GPU as the Python client.
type mt struct {
	state [624]uint32
	idx   int
}

func newMT(seed int) *mt {
	m := &mt{idx: 624}
	m.state[0] = 19650218
	for i := 1; i < 624; i++ {
		m.state[i] = 1812433253*(m.state[i-1]^(m.state[i-1]>>30)) + uint32(i)
	}
	// Python seeds with abs(seed) split into 32-bit words, least significant first
	abs := uint64(seed)
	if seed < 0 {
		abs = -uint64(seed)
	}
	key := []uint32{uint32(abs)}
	if abs>>32 != 0 {
		key = append(key, uint32(abs>>32))
	}
	i, j := 1, 0
	for k := max(624, len(key)); k > 0; k-- {
		m.state[i] = (m.state[i] ^ ((m.state[i-1] ^ (m.state[i-1] >> 30)) * 1664525)) + key[j] + uint32(j)
		i++
		j++
		if i >= 624 {
			m.state[0] = m.state[623]
			i = 1
		}
		if j >= len(key) {
			j = 0
		}
	}
	for k := 623; k > 0; k-- {
		m.state[i] = (m.state[i] ^ ((m.state[i-1] ^ (m.state[i-1] >> 30)) * 1566083941)) - uint32(i)
		i++
		if i >= 624 {
			m.state[0] = m.state[623]
			i = 1
		}
	}
	m.state[0] = 0x80000000
	return m
}

func (m *mt) next() uint32 {
	if m.idx >= 624 {
		for k := 0; k < 624; k++ {
			y := m.state[k]&0x80000000 | m.state[(k+1)%624]&0x7fffffff
			v := m.state[(k+397)%624] ^ y>>1
			if y&1 != 0 {
				v ^= 0x9908b0df
			}
			m.state[k] = v
		}
		m.idx = 0
	}
	y := m.state[m.idx]
	m.idx++
	y ^= y >> 11
	y ^= y << 7 & 0x9d2c5680
	y ^= y << 15 & 0xefc60000
	y ^= y >> 18
	return y
}

// choice is random.choice for a sequence of length n (_randbelow_with_getrandbits).
func (m *mt) choice(n int) int {
	k := bits.Len(uint(n))
	for {
		if r := int(m.next() >> (32 - k)); r < n {
			return r
		}
	}
}

// randomSeed is random.randint(100000, 999999999).
func randomSeed() int {
	n, err := crand.Int(crand.Reader, big.NewInt(999999999-100000+1))
	if err != nil {
		panic(err)
	}
	return int(n.Int64()) + 100000
}
