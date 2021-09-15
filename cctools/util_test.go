package cctools_test

/*
func TestToBinaryString(t *testing.T) {
	bitRanges, err := cctools.LoadControllerBitRanges("../" + cctools.BitRangeFile)
	if err != nil {
		t.Fatal(err)
	}

	randomGen := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 100; i++ {
		rand := make([]byte, 210)
		for j := range rand {
			rand[j] = byte(randomGen.Intn(127))
		}

		for _, c := range cctools.SimpleNd2Ccs {
			bitRange := bitRanges[c]
			bits7 := cctools.ToBoolArray(rand)[bitRange.First : bitRange.Last+1]

			first := bitRange.First + (bitRange.First / 7) + 1
			last := bitRange.Last + (bitRange.Last / 7) + 1

			bin8 := cctools.ToBoolArray8(rand)
			bits8 := bin8[first : last+1] // within a byte
			if first/8 != last/8 {        // different byte
				byteStart := (last / 8) * 8
				bits8 = bin8[first:byteStart]
				bits8 = append(bits8, bin8[byteStart+1:last+1]...)
			}

			if !reflect.DeepEqual(bits7, bits8) {
				t.Errorf("not the same:\n%v\n%v\n", bits7, bits8)
			}

			exp := cctools.ParseControllerByteValue(rand, bitRange)
			val := ParseValue(rand, first, last)
			if exp != val {
				t.Errorf("FAIL: %+v->%v, %v, %v", first, last, exp, val)
			} else {
				t.Logf("PASS: %+v->%v, %v, %v", first, last, exp, val)
			}

		}
	}
}


func ParseValue(msg sysex.SysEx, firstBit, lastBit int) uint8 {
	firstByte := firstBit / 8
	firstBit = firstBit % 8

	lastByte := lastBit / 8
	lastBit = lastBit % 8

	if firstByte == lastByte {
		return msg.Raw()[firstByte] & MsbMaskMap[firstBit] >> (7 - lastBit)
	} else if firstByte+1 == lastByte {
		valMsb := msg.Raw()[lastByte] >> (7 - byte(lastBit))
		valLsb := msg.Raw()[firstByte] & MsbMaskMap[firstBit] << byte(lastBit)
		return valLsb + valMsb
	} else {
		panic("non-contiguous bytes")
	}

}
*/
