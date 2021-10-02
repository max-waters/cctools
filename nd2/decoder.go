package nd2

import (
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"time"

	"github.com/pkg/errors"
)

type Nd2Decoder struct {
	nd2Conn                 *Nd2Connection
	bitRanges               map[uint8]*BitRange
	sysexControllerValueMap map[uint8]map[uint8]uint8
}

func NewNd2Decoder(conf *Nd2ConnectionConfig) (*Nd2Decoder, error) {
	nd2Conn, err := NewNd2Connection(conf)
	if err != nil {
		return nil, err
	}
	return &Nd2Decoder{
		nd2Conn: nd2Conn,
	}, nil
}

func (decoder *Nd2Decoder) Run() error {
	defer decoder.nd2Conn.Close()
	if err := decoder.FindControllerBitRanges(); err != nil {
		return err
	}
	if err := decoder.FindControllerSysExValues(); err != nil {
		return err
	}
	return decoder.Test()
}

func (decoder *Nd2Decoder) FindControllerBitRanges() error {
	fmt.Println("controller,first,last")
	decoder.bitRanges = map[uint8]*BitRange{}
	if err := decoder.findSimpleControllerBitRanges(); err != nil {
		return err
	}
	return decoder.findLsbMsbControllerBitRanges()
}

func (decoder *Nd2Decoder) findSimpleControllerBitRanges() error {
	var c uint8
	for _, c = range SimpleNd2Controllers {
		first, last, err := decoder.findBitRanges(128, func(v uint8) error {
			return decoder.nd2Conn.SendControlChange(0, c, v)
		})
		if err != nil {
			return errors.Wrapf(err, "cannot get bit indexes for controller %d", c)
		}
		if first < 0 {
			fmt.Printf("%d,,\n", c)
			continue
		}
		decoder.bitRanges[c] = &BitRange{Controller: c, First: first, Last: last}
		fmt.Printf("%d,%d,%d\n", c, first, last)
	}
	return nil
}

func (decoder *Nd2Decoder) findLsbMsbControllerBitRanges() error {
	for _, lsbMsb := range Nd2LsbMsbControllers {
		// LSB
		lsbFirst, lsbLast, err := decoder.findBitRanges(128, func(v uint8) error {
			// send LSB of v
			if err := decoder.nd2Conn.SendControlChange(0, lsbMsb[0], v); err != nil {
				return err
			}
			// send MSB of zero
			return decoder.nd2Conn.SendControlChange(0, lsbMsb[1], 0)
		})
		if err != nil {
			return err
		}
		decoder.bitRanges[lsbMsb[0]] = &BitRange{Controller: lsbMsb[0], First: lsbFirst, Last: lsbLast}
		fmt.Printf("%d,%d,%d\n", lsbMsb[0], lsbFirst, lsbLast)

		// MSB
		var max uint8 = 128
		if lsbMsb[0] == EchoBbmController[0] { // special treatment for EchoBpm
			max = 72
		}
		msbFirst, msbLast, err := decoder.findBitRanges(max, func(v uint8) error {
			// send LSB of zero
			if err := decoder.nd2Conn.SendControlChange(0, lsbMsb[0], 0); err != nil {
				return err
			}
			// send MSB of v
			return decoder.nd2Conn.SendControlChange(0, lsbMsb[1], v)
		})
		if err != nil {
			return err
		}
		decoder.bitRanges[lsbMsb[1]] = &BitRange{Controller: lsbMsb[1], First: msbFirst, Last: msbLast}
		fmt.Printf("%d,%d,%d\n", lsbMsb[1], msbFirst, msbLast)
	}
	return nil
}

func (decoder *Nd2Decoder) findBitRanges(max uint8, setFunc func(uint8) error) (int, int, error) {
	// get zero
	if err := decoder.resetCcs(0); err != nil {
		return -1, -1, errors.Wrap(err, "cannot reset control change data")
	}
	zeroProgram, err := decoder.nd2Conn.GetProgram()
	if err != nil {
		return -1, -1, err
	}

	first := math.MaxInt64
	last := -1

	var v uint8
	for v = 0; v < max; v++ {
		if err := setFunc(v); err != nil {
			return -1, -1, err
		}

		program, err := decoder.nd2Conn.GetProgram()
		if err != nil {
			return -1, -1, err
		}
		thisFirst, thisLast := getDifferences(program.GetVoiceBytes(0), zeroProgram.GetVoiceBytes(0))
		if thisFirst < first {
			first = thisFirst
		}
		if thisLast > last {
			last = thisLast
		}
	}
	return first, last, nil
}

func getDifferences(bts1, bts2 []byte) (int, int) {
	firstByte := math.MaxInt64
	for i := 0; i < len(bts1); i++ {
		if bts1[i] != bts2[i] {
			firstByte = i
			break
		}
	}
	if firstByte == math.MaxInt64 {
		return firstByte, -1
	}

	b1 := bts1[firstByte]
	b2 := bts2[firstByte]
	firstBit := math.MaxInt64
	for i := 7; i >= 0; i-- {
		if b1%2 != b2%2 {
			firstBit = i
		}
		b1 = b1 >> 1
		b2 = b2 >> 1
	}

	lastByte := -1
	for i := len(bts1) - 1; i >= firstByte; i-- {
		if bts1[i] != bts2[i] {
			lastByte = i
			break
		}
	}

	b1 = bts1[lastByte]
	b2 = bts2[lastByte]
	lastBit := -1
	for i := 7; i >= 0; i-- {
		if b1%2 != b2%2 {
			lastBit = i
			break
		}
		b1 = b1 >> 1
		b2 = b2 >> 1
	}

	return (firstByte * 8) + firstBit, (lastByte * 8) + lastBit
}

func (decoder *Nd2Decoder) resetCcs(channel uint8) error {
	var c uint8
	for _, c = range SimpleNd2Controllers {
		if err := decoder.nd2Conn.SendControlChange(0, c, 0); err != nil {
			return err
		}
	}
	for _, lsbMsb := range Nd2LsbMsbControllers {
		// LSB then MSB
		if err := decoder.nd2Conn.SendControlChange(0, lsbMsb[0], 0); err != nil {
			return err
		}
		if err := decoder.nd2Conn.SendControlChange(0, lsbMsb[1], 0); err != nil {
			return err
		}
	}
	return nil
}

func (decoder *Nd2Decoder) FindControllerSysExValues() error {
	fmt.Println("controller,controller_value,sysex_value")
	if len(decoder.bitRanges) == 0 {
		decoder.bitRanges = ControllerBitRanges
	}

	if err := decoder.findSimpleControllerSysExValues(); err != nil {
		return err
	}
	return decoder.findLsbMsbControllerSysExValues()
}

func (decoder *Nd2Decoder) findSimpleControllerSysExValues() error {
	for _, c := range SimpleNd2Controllers {
		if err := decoder.findControllerSysExValues(c, func(v uint8) error {
			return decoder.nd2Conn.SendControlChange(0, c, v)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (decoder *Nd2Decoder) findLsbMsbControllerSysExValues() error {
	for _, msbLsb := range Nd2LsbMsbControllers {
		// LSB
		if err := decoder.findControllerSysExValues(msbLsb[0], func(v uint8) error {
			if err := decoder.nd2Conn.SendControlChange(0, msbLsb[0], v); err != nil {
				return err
			}
			return decoder.nd2Conn.SendControlChange(0, msbLsb[1], 0)
		}); err != nil {
			return err
		}

		// MSB
		if err := decoder.findControllerSysExValues(msbLsb[1], func(v uint8) error {
			if err := decoder.nd2Conn.SendControlChange(0, msbLsb[0], 0); err != nil {
				return err
			}
			return decoder.nd2Conn.SendControlChange(0, msbLsb[1], v)
		}); err != nil {
			return err
		}

	}
	return nil
}

func (decoder *Nd2Decoder) findControllerSysExValues(c uint8, setFunc func(v uint8) error) error {
	if err := decoder.resetCcs(0); err != nil {
		return errors.Wrap(err, "cannot reset controller values")
	}
	var v uint8
	for v = 0; v < 128; v++ {
		if err := setFunc(v); err != nil {
			return err
		}

		program, err := decoder.nd2Conn.GetProgram()
		if err != nil {
			return err
		}

		sysExVal := program.GetSysExValue(0, decoder.bitRanges[c])
		fmt.Printf("%d,%d,%d\n", c, v, sysExVal)
	}
	return nil
}

func (decoder *Nd2Decoder) Test() error {
	decoder.sysexControllerValueMap = ControllerValueSysexMap
	decoder.bitRanges = ControllerBitRanges

	randomGen := rand.New(rand.NewSource(time.Now().UnixNano()))
	controllerValues := map[uint8][]uint8{}
	for i := 0; i < 100; i++ {
		fmt.Printf("Test %d\n", i+1)
		var j uint8
		for j = 0; j < 1; j++ {
			// generate and random cc values
			rand := make([]uint8, len(Nd2Controllers))
			for j := 0; j < len(rand); j++ {
				if Nd2Controllers[j] == 29 { // special case, above 71 fucks it
					rand[j] = uint8(randomGen.Intn(72))
				} else {
					rand[j] = uint8(randomGen.Intn(128))
				}
			}
			controllerValues[j] = rand
		}

		normalised, err := decoder.sendAndParseControlChanges(controllerValues)
		if err != nil {
			return err
		}

		reply, err := decoder.sendAndParseControlChanges(normalised)
		if err != nil {
			return err
		}

		if !reflect.DeepEqual(normalised, reply) {
			return errors.Errorf("Unexpected controller values.\nExpected %v\nActual   %v", normalised, reply)
		}
	}
	return nil
}

func (decoder *Nd2Decoder) sendAndParseControlChanges(controllerValues map[uint8][]uint8) (map[uint8][]uint8, error) {
	// send controller values
	for v, values := range controllerValues {
		for i, c := range Nd2Controllers {
			if err := decoder.nd2Conn.SendControlChange(v, c, values[i]); err != nil {
				return nil, err
			}
		}
	}

	// get program sysex
	program, err := decoder.nd2Conn.GetProgram()
	if err != nil {
		return nil, err
	}

	// transform sysex into controller values
	returnValues := map[uint8][]uint8{}
	var v uint8
	for v = 0; v < 1; v++ {
		parsed := make([]uint8, len(Nd2Controllers))
		for j, c := range Nd2Controllers {
			sysExVal := program.GetSysExValue(v, decoder.bitRanges[c])
			controllerVal, ok := decoder.sysexControllerValueMap[c][sysExVal]
			if !ok {
				return nil, errors.Errorf("No value found for controller %d and byte value %d", c, sysExVal)
			}
			parsed[j] = controllerVal
		}
		returnValues[uint8(v)] = parsed
	}

	return returnValues, nil
}

func (decoder *Nd2Decoder) Stop() {
	decoder.nd2Conn.Close()
}
