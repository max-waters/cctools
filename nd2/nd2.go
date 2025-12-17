package nd2

import (
	"log"
	"math/rand"

	"mvw.org/cctools/util"
)

var ControllerNameChanMap = map[string]uint8{
	//
	"filtfreq": 14,
	"ff":       14,
	//
	"filttype": 15,
	//
	"filtenv": 16,
	"fe":      16,
	//
	"filtres": 17,
	"fr":      17,
	//
	"atkrate": 18,
	"na":      18,
	//
	"atkmode": 19,
	//
	"noisedectype": 20,
	//
	"noisedec": 21,
	"nd":       21,
	//
	"noisedeclo": 22,
	//
	"distort": 23,
	"d":       23,
	//
	"disttype": 24,
	"dt":       24,
	//
	"eqfreq": 25,
	"eqf":    25,
	//
	"eqgain": 26,
	"eqg":    26,
	//
	"echofb":     27,
	"echoamount": 28,
	"echoBPMMSB": 29,
	"echoBPMLSB": 61,
	//
	"level": 7,
	"lv":    7,
	//
	"pan": 10,
	//
	"spectra": 30,
	"sp":      30,
	//
	"wave": 46,
	"wv":   46,
	//
	"timbenv":   53,
	"tme":       53,
	"timbre":    52,
	"tm":        52,
	"timbdec":   47,
	"tmd":       47,
	"punch":     48,
	"tonedec":   50,
	"tnd":       50,
	"tonedeclo": 51,
	"tdl":       51,
	//
	"pitchMSB": 31,
	"p":        31,
	//
	"pitchLSB": 63,
	"pq":       63,
	"sclpre":   0,
	"bend":     54,
	"bendtime": 55,
	"bt":       55,
	"clklev":   56,
	"clktype":  57,
	//
	"mix": 58,
	"mx":  58,
	//
	"mutegrp": 59,
	"mg":      59,
}

func GetProgram(conf *Nd2ConnectionConfig, filename string) error {
	conn, err := NewNd2Connection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	program, err := conn.GetProgram()
	if err != nil {
		return err
	}

	voiceControllerValues, err := program.GetVoiceControllerValues()
	if err != nil {
		return err
	}

	filename, err = util.SaveVoiceControllerValues(filename, voiceControllerValues)
	if err != nil {
		return err
	}

	log.Printf("Saved ND2 program to %s\n", filename)
	return nil
}

func SetProgram(conf *Nd2ConnectionConfig, filename string) error {
	conn, err := NewNd2Connection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	values, err := util.LoadVoiceControllerValues(filename)
	if err != nil {
		return err
	}

	for _, value := range values {
		if err := conn.SendControlChange(value.Voice, value.Controller, value.Value); err != nil {
			return err
		}
	}

	log.Printf("Sent program %s to ND2\n", filename)
	return nil
}

func SetVoice(conf *Nd2ConnectionConfig, filename string, voice uint8) error {
	conn, err := NewNd2Connection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	values, err := util.LoadVoiceControllerValues(filename)
	if err != nil {
		return err
	}

	for _, value := range values {
		if value.Voice == voice {
			if err := conn.SendControlChange(value.Voice, value.Controller, value.Value); err != nil {
				return err
			}
		}
	}
	log.Printf("Sent voice %d in program %s to ND2\n", voice+1, filename)
	return nil
}

func CopyVoice(conf *Nd2ConnectionConfig, from, to uint8) error {
	conn, err := NewNd2Connection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	program, err := conn.GetProgram()
	if err != nil {
		return err
	}

	voiceControllerValues, err := program.GetVoiceControllerValues()
	if err != nil {
		return err
	}

	for _, value := range voiceControllerValues {
		if value.Voice == from {
			if err := conn.SendControlChange(to, value.Controller, value.Value); err != nil {
				return err
			}
		}
	}
	log.Printf("Copied voice %d to voice %d in ND2\n", from+1, to+1)
	return nil
}

func SetRandomVoice(conf *Nd2ConnectionConfig, voice uint8, exclude map[uint8]bool) error {
	conn, err := NewNd2Connection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	for _, c := range Nd2Controllers {
		if exclude[c] {
			continue
		}

		if err := conn.SendControlChange(voice, c, uint8(rand.Intn(127))); err != nil {
			return err
		}
	}

	log.Printf("Sent random program to ND2 voice %d\n", voice+1)
	return nil
}
