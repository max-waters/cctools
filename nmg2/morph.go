package nmg2

import (
	"log"
	"sync"
	"time"

	"github.com/max-waters/cctools/util"
	"gitlab.com/gomidi/midi/midimessage/channel"
)

var targetMap map[uint8]uint8 = map[uint8]uint8{0: 0, 19: 1, 37: 2, 55: 3, 73: 4, 91: 5, 109: 6, 127: 7}

const sendPeriod = 500 * time.Millisecond

type NmG2Morpher struct {
	conn           *NmG2Connection
	leftTarget     *util.ControllerValue
	rightTarget    *util.ControllerValue
	morph          *util.ControllerValue
	variations     []map[uint8]uint8 // variation -> controller -? value
	interpolations []map[uint8]uint8 // morph -> controller -> value
	shutdownChan   chan interface{}
	lock           sync.Mutex
}

func NewNmG2Morpher(connConfig *NmG2ConnectionConfig, leftTargetController, rightTargetController, morphController uint8) (*NmG2Morpher, error) {
	conn, err := NewNmG2Connection(connConfig)
	if err != nil {
		return nil, err
	}

	m := &NmG2Morpher{
		conn:           conn,
		leftTarget:     &util.ControllerValue{Controller: leftTargetController},
		rightTarget:    &util.ControllerValue{Controller: rightTargetController},
		morph:          &util.ControllerValue{Controller: morphController},
		interpolations: make([]map[uint8]uint8, 128),
		variations:     make([]map[uint8]uint8, 8),
		shutdownChan:   make(chan interface{}, 1),
		lock:           sync.Mutex{},
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

	for i, variation := range variations {
		for _, cv := range variation {
			// do not not morph the target/morph/variation controllers
			// also get the current value for these controllers in variation 0
			if cv.Controller == m.leftTarget.Controller {
				if i == 0 {
					m.leftTarget.Value = cv.Value
				}
				continue
			}
			if cv.Controller == m.rightTarget.Controller {
				if i == 0 {
					m.rightTarget.Value = cv.Value
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

	// compute initial interpolations
	m.InitInterpolations()

	// new thread
	go func() {
		ticker := time.NewTicker(sendPeriod)
		for {
			select {
			case <-m.shutdownChan:
				return
			case <-ticker.C:
				if err := m.sendUpdate(); err != nil {
					log.Printf("Error sending control changes: %s\n", err)
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
		if c == m.leftTarget.Controller || c == m.rightTarget.Controller ||
			c == m.morph.Controller || c == VarChangeController {
			panic(c)
		}
		if err := m.conn.SendControlChange(c, v); err != nil {
			return err
		}
	}
	return nil
}

func (m *NmG2Morpher) ProcessControlChange(c *channel.ControlChange) error {
	if c.Controller() == m.leftTarget.Controller {
		m.leftTarget.Value = c.Value()
		m.SetInterpolations()
	} else if c.Controller() == m.rightTarget.Controller {
		m.rightTarget.Value = c.Value()
		m.SetInterpolations()
	} else if c.Controller() == m.morph.Controller {
		log.Printf("Morph: %d\n", c.Value())
		m.morph.Value = c.Value()
	}
	return nil
}

func (m *NmG2Morpher) InitInterpolations() {
	m.lock.Lock()
	defer m.lock.Unlock()

	// NB assumes that G2 only sends exact numbers -- 0, 19, 37 etc
	leftTarget := targetMap[m.leftTarget.Value]
	rightTarget := targetMap[m.rightTarget.Value]

	// calculate interpolations from left target to right target
	for c, leftTargetVal := range m.variations[leftTarget] {
		rightTargetVal := m.variations[rightTarget][c]
		stepDiff := float64(rightTargetVal-leftTargetVal) / 128
		for i := 0; i < 128; i++ {
			m.interpolations[i][c] = leftTargetVal + uint8(stepDiff*float64(i))
		}
	}
}

func (m *NmG2Morpher) SetInterpolations() {
	m.lock.Lock()
	defer m.lock.Unlock()

	// NB assumes that G2 only sends exact numbers -- 0, 19, 37 etc
	leftTarget := targetMap[m.leftTarget.Value]
	rightTarget := targetMap[m.rightTarget.Value]
	log.Printf("Target: %d<->%d\n", leftTarget, rightTarget)
	// calculate interpolations from left target to current value
	for c, v := range m.variations[leftTarget] {
		current := m.interpolations[m.morph.Value][c]
		stepDiff := float64(current-v) / float64(m.morph.Value)
		for i := 0; i < int(m.morph.Value); i++ {
			m.interpolations[i][c] = v + uint8(stepDiff*float64(i))
		}
	}

	// calculate interpolations from current value to right target
	for c, v := range m.variations[rightTarget] {
		current := m.interpolations[m.morph.Value][c]
		stepDiff := float64(v-current) / float64(128-m.morph.Value)
		for i := (m.morph.Value + 1); i < 128; i++ {
			m.interpolations[i][c] = current + uint8(stepDiff*float64(i))
		}
	}
}

func (m *NmG2Morpher) Close() {
	m.conn.Close()
}
