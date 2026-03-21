package nmg2

import (
	"log"

	"github.com/max-waters/cctools/util"
)

func GetVariations(conf *NmG2ConnectionConfig, filename string, maxMspFormat bool) error {
	conn, err := NewNmG2Connection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	variations, err := conn.GetVariations()
	if err != nil {
		return err
	}

	vlist := []*util.VoiceControllerValue{}
	var v uint8
	for v = 0; v < 8; v++ {
		for _, vc := range variations[v] {
			vlist = append(vlist, &util.VoiceControllerValue{Voice: v, Controller: vc.Controller, Value: vc.Value})
		}
	}

	var f func(filename string, data []*util.VoiceControllerValue) (string, error)
	if maxMspFormat {
		f = util.SaveVoiceControllerValuesAsMaxMsp
	} else {
		f = util.SaveVoiceControllerValues
	}
	filename, err = f(filename, vlist)
	if err != nil {
		return err
	}
	log.Printf("Saved NmG2 variations for voice %s to %s\n", conf.Voice, filename)
	return nil
}
