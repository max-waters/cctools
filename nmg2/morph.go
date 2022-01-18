package nmg2

import (
	"gitlab.com/gomidi/midi/midimessage/channel"
	"mvw.org/cctools/util"
)

type NmG2Morpher struct {
	conn             *NmG2Connection
	targetController uint8
	morph            *util.ControllerValue
	variations       []map[uint8]uint8 // variation -> controller -? value
	interpolations   []map[uint8]uint8 // morph -> controller -> value
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
	}

	return m, nil
}

func (m *NmG2Morpher) Start() error {
	// get variations
	variations, err := m.conn.GetVariations()
	if err != nil {
		return err
	}
	for i, variation := range variations {
		for _, cv := range variation {
			if cv.Controller != m.targetController && cv.Controller != m.morph.Controller {
				m.variations[i][cv.Controller] = cv.Value
			}
		}
	}

	// reset variation controller
	if err := m.conn.SetVariation(0); err != nil {
		return err
	}

	// reset target controller to variation 1
	if err := m.conn.SendControlChange(m.targetController, 16); err != nil {
		return err
	}

	// compute interpolation to variation 1, set morph to 0
	if err := m.SetTarget(1); err != nil {
		return err
	}

	// blocks until m.conn.Close() is called
	m.conn.ListenForControlChanges(m.ProcessControlChange)

	return nil
}

func (m *NmG2Morpher) ProcessControlChange(c *channel.ControlChange) {
	if c.Controller() == m.targetController {
		m.SetTarget(c.Value() / 16) // controller range divided equally into 8 regions
	} else if c.Controller() == m.morph.Controller {
		m.SetMorph(c.Value())
	}
}

func (m *NmG2Morpher) SetTarget(target uint8) error {
	// calculate interpolations from current value to target
	for c, v := range m.variations[target] {
		current := m.interpolations[m.morph.Value][c]
		stepDiff := float64(v-current) / 128
		for i := 0; i < 128; i++ {
			m.interpolations[i][c] = current + uint8(stepDiff*float64(i))
		}
	}

	// set morph back to zero
	if err := m.conn.SendControlChange(m.morph.Controller, 0); err != nil {
		return err
	}

	return nil
}

func (m *NmG2Morpher) SetMorph(morphValue uint8) error {
	for c, v := range m.interpolations[morphValue] {
		m.conn.SendControlChange(c, v)
	}
	return nil
}

func (m *NmG2Morpher) Close() {
	m.conn.Close()
}
