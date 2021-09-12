package cctools_test

import (
	"testing"

	"mvw.org/cctools/cctools"
)

func TestToBinaryString(t *testing.T) {
	type testCase struct {
		u uint8
		s string
	}

	testCases := []testCase{
		{u: 0, s: "00000000"},
		{u: 1, s: "00000001"},
		{u: 2, s: "00000010"},
		{u: 10, s: "00001010"},
		{u: 128, s: "10000000"},
		{u: 129, s: "10000001"},
	}

	for _, tc := range testCases {
		if s := cctools.ToBinaryString(tc.u); s != tc.s {
			t.Errorf("%+v, got %s", tc, s)
		}
	}
}
