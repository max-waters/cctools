package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gocarina/gocsv"
	"mvw.org/cctools/util"
)

func main() {
	fileName := os.Args[1]
	vcvs, err := PercColl2Vcv(fileName)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	str, err := gocsv.MarshalString(vcvs)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Print(str)
}

func PercColl2Vcv(filename string) ([]*util.VariationVoiceControllerValue, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vcvs := []*util.VariationVoiceControllerValue{}
	fileScanner := bufio.NewScanner(f)
	for fileScanner.Scan() {
		// voice, channel, var_0_val, var_1_val ...
		// 0 37 0 93 93
		// 0 72 72 72 0
		spl := strings.Split(strings.TrimSpace(fileScanner.Text()), " ")
		voice, err := strconv.ParseUint(spl[0], 10, 8)
		if err != nil {
			return nil, err
		}
		ctrlr, err := strconv.ParseUint(spl[1], 10, 8)
		if err != nil {
			return nil, err
		}

		for i := 2; i < len(spl); i++ {
			val, err := strconv.ParseUint(spl[i], 10, 8)
			if err != nil {
				return nil, err
			}

			vcvs = append(vcvs, &util.VariationVoiceControllerValue{
				Variation:  uint8(i) - 2,
				Voice:      uint8(voice),
				Controller: uint8(ctrlr),
				Value:      uint8(val),
			})
		}
	}

	sort.Slice(vcvs, func(i, j int) bool {
		if vcvs[i].Variation == vcvs[j].Variation {
			if vcvs[i].Voice == vcvs[j].Voice {
				return vcvs[i].Controller < vcvs[j].Controller
			}
			return vcvs[i].Voice < vcvs[j].Voice
		}
		return vcvs[i].Variation < vcvs[j].Variation
	})

	return vcvs, nil
}

func Coll2Vcv(filename string) ([]*util.VariationControllerValue, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vcvs := []*util.VariationControllerValue{}
	fileScanner := bufio.NewScanner(f)
	for fileScanner.Scan() {
		// channel, vc_0_val vc_1_val ... ;
		// 4, 74 74 52 52 52 74 74 74 ;
		spl := strings.Split(fileScanner.Text(), ",")
		ctlr, err := strconv.ParseUint(spl[0], 10, 8)
		if err != nil {
			return nil, err
		}

		vcVals := strings.Split(strings.TrimSpace(spl[1]), " ")
		vcVals = vcVals[:len(vcVals)-1] // ignore ';'

		for vc, valStr := range vcVals {
			val, err := strconv.ParseUint(valStr, 10, 8)
			if err != nil {
				return nil, err
			}

			vcvs = append(vcvs, &util.VariationControllerValue{
				Variation:  uint8(vc),
				Controller: uint8(ctlr),
				Value:      uint8(val),
			})
		}
	}

	sort.Slice(vcvs, func(i, j int) bool {
		if vcvs[i].Variation == vcvs[j].Variation {
			return vcvs[i].Controller < vcvs[j].Controller
		}
		return vcvs[i].Variation < vcvs[j].Variation
	})

	return vcvs, nil
}
