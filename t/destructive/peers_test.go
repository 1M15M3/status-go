package destructive

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/whisper/whisperv6"
	"github.com/status-im/status-go/geth/api"
	. "github.com/status-im/status-go/t/utils"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/stretchr/testify/suite"
)

const (
	defaultTimeout = 40 * time.Second
)

func TestPeersSuiteNetworkConnection(t *testing.T) {
	suite.Run(t, &PeersTestSuite{controller: new(NetworkConnectionController)})
}

type PeersTestSuite struct {
	suite.Suite

	backend    *api.StatusBackend
	controller *NetworkConnectionController
}

func (s *PeersTestSuite) SetupTest() {
	s.backend = api.NewStatusBackend()
	config, err := MakeTestNodeConfig(GetNetworkID())
	s.Require().NoError(err)
	// we need to enable atleast 1 protocol, otherwise peers won't connect
	config.LightEthConfig.Enabled = false
	config.WhisperConfig.Enabled = true
	s.Require().NoError(s.backend.StartNode(config))
}

func (s *PeersTestSuite) TearDownTest() {
	s.Require().NoError(s.backend.StopNode())
}

func consumeUntil(events <-chan *p2p.PeerEvent, f func(ev *p2p.PeerEvent) bool, timeout time.Duration) error {
	timer := time.After(timeout)
	for {
		select {
		case ev := <-events:
			if f(ev) {
				return nil
			}
		case <-timer:
			return errors.New("timeout")
		}
	}
}

func (s *PeersTestSuite) TestSentEnvelope() {
	node := s.backend.StatusNode()
	w, err := node.WhisperService()
	s.NoError(err)
	events := make(chan whisperv6.EnvelopeEvent, 100)
	sub := w.SubscribeEnvelopeEvents(events)
	defer sub.Unsubscribe()

	client, _ := node.GethNode().Attach()
	s.NotNil(client)
	var symID string
	s.NoError(client.Call(&symID, "shh_newSymKey"))
	msg := whisperv6.NewMessage{
		SymKeyID:  symID,
		PowTarget: whisperv6.DefaultMinimumPoW,
		PowTime:   200,
		TTL:       1,
		Topic:     whisperv6.TopicType{0x01, 0x01, 0x01, 0x01},
		Payload:   []byte("hello"),
	}
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				var hash common.Hash
				s.NoError(client.Call(&hash, "shhext_post", msg))
				fmt.Println("SEND ENVELOPE", hash)
			}
		}
	}()

	wait := func() {
		for {
			select {
			case ev := <-events:
				if ev.Event == whisperv6.EventEnvelopeSent {
					fmt.Println("******* ENVELOPE SENT", ev.Hash)
					return
				} else {
					fmt.Println("****** ENVELOPE EXPIRED", ev.Hash)
				}
			}
		}
	}
	wait()
	go func() {
		for {
			select {
			case ev := <-events:
				fmt.Println("===", ev.Event, ev.Hash, time.Now())
			}
		}
	}()
	s.Require().NoError(s.controller.Enable())
	fmt.Println("---------------- connectivity dropped", time.Now())
	time.Sleep(120 * time.Second)
}

// TestStaticPeersReconnect : it tests how long it takes to reconnect with
// peers after losing connection. This is something we will have to support
// in order for mobile devices to reconnect fast if network connectivity
// is lost for ~30s.
func (s *PeersTestSuite) TestStaticPeersReconnect() {
	// both on rinkeby and ropsten we can expect atleast 2 peers connected
	expectedPeersCount := 2
	events := make(chan *p2p.PeerEvent, 10)
	node := s.backend.StatusNode().GethNode()
	s.Require().NotNil(node)

	subscription := node.Server().SubscribeEvents(events)
	defer subscription.Unsubscribe()
	peers := map[discover.NodeID]struct{}{}
	before := time.Now()
	s.Require().NoError(consumeUntil(events, func(ev *p2p.PeerEvent) bool {
		log.Info("tests", "event", ev)
		if ev.Type == p2p.PeerEventTypeAdd {
			peers[ev.Peer] = struct{}{}
		}
		return len(peers) == expectedPeersCount
	}, defaultTimeout))
	s.WithinDuration(time.Now(), before, 5*time.Second)

	s.Require().NoError(s.controller.Enable())
	before = time.Now()

	s.Require().NoError(consumeUntil(events, func(ev *p2p.PeerEvent) bool {
		log.Info("tests", "event", ev)
		if ev.Type == p2p.PeerEventTypeDrop {
			delete(peers, ev.Peer)
		}
		return len(peers) == 0
	}, defaultTimeout))
	s.WithinDuration(time.Now(), before, 31*time.Second)

	s.Require().NoError(s.controller.Disable())
	before = time.Now()
	go func() {
		s.NoError(s.backend.StatusNode().ReconnectStaticPeers())
	}()
	s.Require().NoError(consumeUntil(events, func(ev *p2p.PeerEvent) bool {
		log.Info("tests", "event", ev)
		if ev.Type == p2p.PeerEventTypeAdd {
			peers[ev.Peer] = struct{}{}
		}
		return len(peers) == expectedPeersCount
	}, defaultTimeout))
	s.WithinDuration(time.Now(), before, 31*time.Second)
}
