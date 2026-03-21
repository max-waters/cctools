package util_test

import (
	"strconv"
	"testing"

	"github.com/max-waters/cctools/util"
)

func TestGetNumberedFileRegex(t *testing.T) {
	type testCase struct {
		FileBase, Extension, Filename string
		ExpectedNum                   int
	}
	testCases := []*testCase{
		{FileBase: "name", Extension: ".csv", Filename: "name-100.csv", ExpectedNum: 100},
		{FileBase: "name", Extension: "csv", Filename: "name-1.csv", ExpectedNum: 1},
		{FileBase: "name", Extension: ".csv", Filename: "name-0010.csv", ExpectedNum: 10},
		{FileBase: "name", Extension: "csv", Filename: "name.csv", ExpectedNum: -1},
		{FileBase: "name", Extension: ".csv", Filename: "blah-100.csv", ExpectedNum: -1},
		{FileBase: "name", Extension: "csv", Filename: "name-100.csv2", ExpectedNum: -1},
		{FileBase: "name", Extension: ".csv", Filename: "prefixname-100.csv", ExpectedNum: -1},
		{FileBase: "name", Extension: ".csv", Filename: "name-100-11.csv", ExpectedNum: -1},
		{FileBase: "name++", Extension: ".csv", Filename: "name++-100.csv", ExpectedNum: 100},
		{FileBase: "name-(something)", Extension: ".csv", Filename: "name-(something)-100.csv", ExpectedNum: 100},
		{FileBase: "$[name]-(***)", Extension: ".c^v", Filename: "$[name]-(***)-100.c^v", ExpectedNum: 100},
	}

	for _, testCase := range testCases {
		//compile
		regex, err := util.GetNumberedFileRegex(testCase.FileBase, testCase.Extension)
		if err != nil {
			t.Errorf("error compiling regex: %s", err)
			continue
		}

		// match
		expMatch := testCase.ExpectedNum != -1
		matches := regex.FindAllStringSubmatch(testCase.Filename, -1)
		matched := len(matches) > 0
		if matched != expMatch {
			t.Errorf("regex match for file %s: %v, expected %v", testCase.Filename, matched, expMatch)
			continue
		}

		// get number
		if expMatch {
			if len(matches) != 1 || len(matches[0]) != 2 {
				t.Errorf("unexpected number of submatches: %v", matches)
			}
			n, err := strconv.Atoi(matches[0][1])
			if err != nil {
				t.Errorf("cannot parse submatch as number: %v", matches[0][1])
			}
			if n != testCase.ExpectedNum {
				t.Errorf("expected file number %d, got %d", testCase.ExpectedNum, n)
			}
		}
	}
}
