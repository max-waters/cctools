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
	shutdownChan            chan interface{}
	bitRanges               map[uint8]*BitRange
	sysexControllerValueMap map[uint8]map[uint8]uint8
}

func NewNd2Decoder(inPort, outPort uint, outChan uint8) (*Nd2Decoder, error) {
	nd2Conn, err := NewNd2Connection(inPort, outPort, outChan)
	if err != nil {
		return nil, err
	}
	return &Nd2Decoder{
		nd2Conn:      nd2Conn,
		shutdownChan: make(chan interface{}, 1),
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
			return decoder.nd2Conn.SendControlChange(c, v)
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
			if err := decoder.nd2Conn.SendControlChange(lsbMsb[0], v); err != nil {
				return err
			}
			// send MSB of zero
			return decoder.nd2Conn.SendControlChange(lsbMsb[1], 0)
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
			if err := decoder.nd2Conn.SendControlChange(lsbMsb[0], 0); err != nil {
				return err
			}
			// send MSB of v
			return decoder.nd2Conn.SendControlChange(lsbMsb[1], v)
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
	if err := decoder.resetCcs(); err != nil {
		return -1, -1, errors.Wrap(err, "cannot reset control change data")
	}
	if err := decoder.nd2Conn.SendProgramRequest(); err != nil {
		return -1, -1, err
	}
	zeroSysExMsg := <-decoder.nd2Conn.responseChan
	zeroProgram := Nd2ProgramSysex(zeroSysExMsg)

	first := math.MaxInt64
	last := -1

	var v uint8
	for v = 0; v < max; v++ {
		if err := setFunc(v); err != nil {
			return -1, -1, err
		}

		if err := decoder.nd2Conn.SendProgramRequest(); err != nil {
			return -1, -1, err
		}

		select {
		case <-decoder.shutdownChan:
			return -1, -1, errors.New("cancelled")
		case sysExMsg := <-decoder.nd2Conn.responseChan:
			program := Nd2ProgramSysex(sysExMsg)
			thisFirst, thisLast := getDifferences(program.GetVoiceBytes(0), zeroProgram.GetVoiceBytes(0))
			if thisFirst < first {
				first = thisFirst
			}
			if thisLast > last {
				last = thisLast
			}
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

func (decoder *Nd2Decoder) resetCcs() error {
	var c uint8
	for _, c = range SimpleNd2Controllers {
		if err := decoder.nd2Conn.SendControlChange(c, 0); err != nil {
			return err
		}
	}
	for _, lsbMsb := range Nd2LsbMsbControllers {
		// LSB then MSB
		if err := decoder.nd2Conn.SendControlChange(lsbMsb[0], 0); err != nil {
			return err
		}
		if err := decoder.nd2Conn.SendControlChange(lsbMsb[1], 0); err != nil {
			return err
		}
	}
	return nil
}

func (decoder *Nd2Decoder) FindControllerSysExValues() error {
	fmt.Println("controller,controller_value,sysex_value")
	if len(decoder.bitRanges) == 0 {
		bitRanges, err := LoadControllerBitRanges()
		if err != nil {
			return err
		}
		decoder.bitRanges = bitRanges
	}

	if err := decoder.findSimpleControllerSysExValues(); err != nil {
		return err
	}
	return decoder.findLsbMsbControllerSysExValues()
}

func (decoder *Nd2Decoder) findSimpleControllerSysExValues() error {
	for _, c := range SimpleNd2Controllers {
		if err := decoder.findControllerSysExValues(c, func(v uint8) error {
			return decoder.nd2Conn.SendControlChange(c, v)
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
			if err := decoder.nd2Conn.SendControlChange(msbLsb[0], v); err != nil {
				return err
			}
			return decoder.nd2Conn.SendControlChange(msbLsb[1], 0)
		}); err != nil {
			return err
		}

		// MSB
		if err := decoder.findControllerSysExValues(msbLsb[1], func(v uint8) error {
			if err := decoder.nd2Conn.SendControlChange(msbLsb[0], 0); err != nil {
				return err
			}
			return decoder.nd2Conn.SendControlChange(msbLsb[1], v)
		}); err != nil {
			return err
		}

	}
	return nil
}

func (decoder *Nd2Decoder) findControllerSysExValues(c uint8, setFunc func(v uint8) error) error {
	if err := decoder.resetCcs(); err != nil {
		return errors.Wrap(err, "cannot reset controller values")
	}
	var v uint8
	for v = 0; v < 128; v++ {
		if err := setFunc(v); err != nil {
			return err
		}

		if err := decoder.nd2Conn.SendProgramRequest(); err != nil {
			return err
		}

		select {
		case <-decoder.shutdownChan:
			return errors.New("cancelled")
		case sysExMsg := <-decoder.nd2Conn.responseChan:
			program := Nd2ProgramSysex(sysExMsg)
			controllerRange := decoder.bitRanges[c]
			sysExVal := program.GetSysExValue(controllerRange.First+(HeaderBytes*8), controllerRange.Last+(HeaderBytes*8))
			fmt.Printf("%d,%d,%d\n", c, v, sysExVal)
		}
	}
	return nil
}

func (decoder *Nd2Decoder) Test() error {
	controllerValueMap, err := LoadSysexControllerValueMap()
	if err != nil {
		return err
	}
	decoder.sysexControllerValueMap = controllerValueMap

	controllerBitRanges, err := LoadControllerBitRanges()
	if err != nil {
		return err
	}
	decoder.bitRanges = controllerBitRanges

	randomGen := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < 100; i++ {
		fmt.Printf("Test %d\n", i+1)
		// generate and send random cc values
		rand := make([]uint8, len(Nd2Controllers))
		for j := 0; j < len(rand); j++ {
			if Nd2Controllers[j] == 29 { // special case, above 71 fucks it
				rand[j] = uint8(randomGen.Intn(72))
			} else {
				rand[j] = uint8(randomGen.Intn(128))
			}
		}

		normalised, err := decoder.sendAndParseControlChanges(rand)
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

func (decoder *Nd2Decoder) sendAndParseControlChanges(values []uint8) ([]uint8, error) {
	for i, c := range Nd2Controllers {
		if err := decoder.nd2Conn.SendControlChange(c, values[i]); err != nil {
			return nil, err
		}
	}

	if err := decoder.nd2Conn.SendProgramRequest(); err != nil {
		return nil, err
	}

	select {
	case <-decoder.shutdownChan:
		return nil, errors.New("cancelled")
	case sysExMsg := <-decoder.nd2Conn.responseChan:
		// transform sysex into cc values
		parsed := make([]uint8, len(Nd2Controllers))
		sysExVals := make([]uint8, len(Nd2Controllers))
		for j, c := range Nd2Controllers {
			program := Nd2ProgramSysex(sysExMsg)
			controllerRange := decoder.bitRanges[c]
			sysExVal := program.GetSysExValue(controllerRange.First+(HeaderBytes*8), controllerRange.Last+(HeaderBytes*8))
			sysExVals[j] = sysExVal
			cc, ok := decoder.sysexControllerValueMap[c][sysExVal]
			if !ok {
				return nil, errors.Errorf("No value found for controller %d and byte value %d", c, sysExVal)
			}
			parsed[j] = cc
		}

		return parsed, nil
	}
}

func (decoder *Nd2Decoder) Stop() {
	decoder.shutdownChan <- nil
}
