package nmg2

import (
	"fmt"
	"sync"
	"time"

	"gitlab.com/gomidi/midi/midimessage/channel"
	"mvw.org/cctools/util"
)

var targetMap map[uint8]uint8 = map[uint8]uint8{0: 0, 19: 1, 37: 2, 55: 3, 73: 4, 91: 5, 109: 6, 127: 7}

const sendPeriod = 500 * time.Millisecond

type NmG2Morpher struct {
	conn             *NmG2Connection
	targetController uint8
	morph            *util.ControllerValue
	variations       []map[uint8]uint8 // variation -> controller -? value
	interpolations   []map[uint8]uint8 // morph -> controller -> value
	shutdownChan     chan interface{}
	lock             sync.Mutex
}

func NewNmG2Morpher(connConfig *NmG2ConnectionConfig, targetController, morphController uint8) (*NmG2Morpher, error) {
	conn, err := NewNmG2Connection(connConfig)
	if err != nil {
		return nil, err
	}

	m := &NmG2Morpher{
		conn:             conn,
		targetController: targetController,
		morph:            &util.ControllerValue{Controller: morphController, Value: 0},
		interpolations:   make([]map[uint8]uint8, 128),
		variations:       make([]map[uint8]uint8, 8),
		shutdownChan:     make(chan interface{}, 1),
		lock:             sync.Mutex{},
	}
	for i := 0; i < len(m.interpolations); i++ {
		m.interpolations[i] = map[uint8]uint8{}
	}
	for i := 0; i < len(m.variations); i++ {
		m.variations[i] = map[uint8]uint8{}
	}

	return m, nil
}

func (m *NmG2Morpher) Start() error {
	// get variations
	variations, err := m.conn.GetVariations()
	if err != nil {
		return err
	}
	var currentTarget uint8
	for i, variation := range variations {
		for _, cv := range variation {
			// do not not morph the target/morph/variation controllers
			if cv.Controller == m.targetController {
				if i == 0 {
					currentTarget = cv.Value
				}
				continue
			}
			if cv.Controller == m.morph.Controller {
				if i == 0 {
					m.morph.Value = cv.Value
				}
				continue
			}
			if cv.Controller == VarChangeController {
				continue
			}

			m.variations[i][cv.Controller] = cv.Value
		}
	}

	// reset to variation 0
	if err := m.conn.SetVariation(0); err != nil {
		return err
	}

	// compute interpolation to target value in variation 0, set morph to 0
	if err := m.SetTarget(targetMap[currentTarget]); err != nil {
		return err
	}

	// new thread
	go func() {
		ticker := time.NewTicker(sendPeriod)
		for {
			select {
			case <-m.shutdownChan:
				return
			case <-ticker.C:
				if err := m.sendUpdate(); err != nil {
					fmt.Printf("Error sending control changes: %s\n", err)
					return
				}
			}
		}
	}()

	// blocks until m.conn.Close() is called or
	// m.ProcessControlChange() returns an error
	return m.conn.ListenForControlChanges(m.ProcessControlChange)
}

func (m *NmG2Morpher) sendUpdate() error {
	m.lock.Lock()
	defer m.lock.Unlock()
	for c, v := range m.interpolations[m.morph.Value] {
		if c == m.targetController || c == m.morph.Controller || c == VarChangeController {
			panic(c)
		}
		if err := m.conn.SendControlChange(c, v); err != nil {
			return err
		}
	}
	return nil
}

func (m *NmG2Morpher) ProcessControlChange(c *channel.ControlChange) error {
	if c.Controller() == m.targetController {
		// NB assumes that G2 only sends exact numbers -- 0, 19, 37 etc
		return m.SetTarget(targetMap[c.Value()])
	}
	if c.Controller() == m.morph.Controller {
		return m.SetMorph(c.Value())
	}
	return nil
}

func (m *NmG2Morpher) SetTarget(target uint8) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	fmt.Printf("target: %d\n", target)
	// calculate interpolations from current value to target
	for c, v := range m.variations[target] {
		current := m.interpolations[m.morph.Value][c]
		stepDiff := float64(v-current) / 128
		for i := 0; i < 128; i++ {
			m.interpolations[i][c] = current + uint8(stepDiff*float64(i))
		}
	}

	//set morph back to zero
	m.morph.Value = 0
	if err := m.conn.SendControlChange(m.morph.Controller, 0); err != nil {
		return err
	}

	return nil
}

func (m *NmG2Morpher) SetMorph(morphValue uint8) error {
	fmt.Printf("morph: %d\n", morphValue)
	m.morph.Value = morphValue
	return nil
}

func (m *NmG2Morpher) Close() {
	m.conn.Close()
}
