package iplist

import (
	"github.com/stretchr/testify/assert"
	"testing"

	"net"
)

func TestParseAddress(t *testing.T) {
	validTests := []struct {
		a string
		n *net.IPNet
	}{
		{
			a: "192.168.0.0/24",
			n: &net.IPNet{
				IP:   []byte{192, 168, 0, 0},
				Mask: net.CIDRMask(24, 32),
			},
		},
		{
			a: "127.0.0.1",
			n: &net.IPNet{
				IP:   []byte{127, 0, 0, 1},
				Mask: net.CIDRMask(32, 32),
			},
		},
	}

	invalidTests := []struct {
		a string
	}{
		{
			a: "192.168.0.0/33",
		},
		{
			a: "127.0.0.256",
		},
	}

	for _, test := range validTests {
		t.Run(test.a, func(t *testing.T) {
			n, err := parseAddress(test.a)
			if assert.Nil(t, err) {
				assert.Equal(t, n, test.n)
			}
		})
	}

	for _, test := range invalidTests {
		t.Run(test.a, func(t *testing.T) {
			_, err := parseAddress(test.a)
			assert.NotNil(t, err)
		})
	}
}
