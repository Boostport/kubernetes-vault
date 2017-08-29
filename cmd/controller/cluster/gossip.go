package cluster

import (
	"io"
	"strconv"

	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/serf/serf"
	"github.com/pkg/errors"
)

type Gossip struct {
	*serf.Serf
	port   int
	events chan serf.Event
}

func (g *Gossip) Events() <-chan serf.Event {
	return g.events
}

func NewGossip(bindAddr string, join []string, port int, logOutput io.Writer) (*Gossip, error) {

	if port <= 0 {
		port = defaultGossipPort
	}

	events := make(chan serf.Event, 16)

	memberlistConfig := memberlist.DefaultLANConfig()
	memberlistConfig.BindAddr = bindAddr
	memberlistConfig.BindPort = port
	memberlistConfig.LogOutput = logOutput

	serfConfig := serf.DefaultConfig()
	serfConfig.NodeName = bindAddr + ":" + strconv.Itoa(port)
	serfConfig.EventCh = events
	serfConfig.MemberlistConfig = memberlistConfig
	serfConfig.LogOutput = logOutput

	s, err := serf.Create(serfConfig)

	if err != nil {
		return nil, errors.Wrap(err, "failed to create memberlist")
	}

	// Join an existing cluster by specifying at least one known member.
	if len(join) > 0 {
		_, err = s.Join(join, false)
		if err != nil {
			return nil, errors.Wrap(err, "failed to join cluster")
		}
	}

	return &Gossip{
		Serf:   s,
		port:   port,
		events: events,
	}, nil
}
