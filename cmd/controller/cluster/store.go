package cluster

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/Boostport/kubernetes-vault/cmd/controller/client"
	"github.com/cenkalti/backoff"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/raft"
	"github.com/hashicorp/raft-boltdb"
	"github.com/hashicorp/serf/serf"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

type snapshot struct{}

func (s *snapshot) Persist(sink raft.SnapshotSink) error {
	// no-op
	return nil
}

func (s *snapshot) Release() {
	// no-op
}

type Config struct {
	Logger            *logrus.Logger
	PollPodsFrequency time.Duration
}

func DefaultStoreConfig() Config {

	return Config{
		Logger:            &logrus.Logger{},
		PollPodsFrequency: defaultPollPodsFrequency,
	}
}

type Store struct {
	Raft           *raft.Raft
	gossip         *Gossip
	config         Config
	kubeClient     *client.Kube
	vaultClient    *client.Vault
	httpClient     *http.Client
	peerStore      raft.PeerStore
	logger         *logrus.Logger
	shutdownLeader chan struct{}
	shutdown       chan struct{}

	sync.Mutex
	pods map[string]client.Pod
}

func (s *Store) Apply(l *raft.Log) interface{} {
	// no-op
	return nil
}

func (s *Store) Restore(snap io.ReadCloser) error {
	// no-op
	return nil
}

func (s *Store) Snapshot() (raft.FSMSnapshot, error) {
	// no-op
	return &snapshot{}, nil
}

func (s *Store) StartRaft(dataDir string, bindAddr string, logOutput io.Writer) error {

	port := s.gossip.port + 1

	raftDBPath := filepath.Join(dataDir, "raft.db")

	raftDB, err := raftboltdb.NewBoltStore(raftDBPath)

	if err != nil {
		return errors.Wrap(err, "unable to create bolt store")
	}

	snapshotStore, err := raft.NewFileSnapshotStore(dataDir, 1, logOutput)

	if err != nil {
		return errors.Wrap(err, "unable to create snapshot store")
	}

	trans, err := raft.NewTCPTransport(bindAddr+":"+strconv.Itoa(port), nil, 3, 5*time.Second, logOutput)

	if err != nil {
		return errors.Wrap(err, "unable to create transport")
	}

	peerStore := raft.NewJSONPeers(dataDir, trans)

	c := raft.DefaultConfig()
	c.EnableSingleNode = true
	c.DisableBootstrapAfterElect = false
	c.ShutdownOnRemove = false
	c.LogOutput = logOutput

	r, err := raft.NewRaft(c, s, raftDB, raftDB, snapshotStore, peerStore, trans)

	// Force set peers (to prevent stale peers) on start up
	peers := []string{}

	for _, node := range s.gossip.Members() {
		peers = append(peers, node.Addr.String()+":"+strconv.Itoa(int(node.Port+1)))
	}

	r.SetPeers(peers)

	s.Raft = r
	s.peerStore = peerStore

	if err != nil {
		return errors.Wrap(err, "failed to create raft")
	}

	go s.start()

	return nil
}

func (s *Store) start() {

	for {
		select {
		case <-s.shutdown:

			if s.shutdownLeader != nil {
				close(s.shutdownLeader)
			}

			f := s.Raft.Shutdown()

			if f.Error() != nil {
				s.logger.Errorf("Could not shutdown raft: %s", f.Error())
			}

			err := s.gossip.Shutdown()

			if err != nil {
				s.logger.Errorf("Could not shutdown memberlist: %s", err)
			}

			return

		case event := <-s.gossip.Events():

			if memberEvent, ok := event.(serf.MemberEvent); ok {
				s.handleGossipMembershipChange(memberEvent)
			}

		case l := <-s.Raft.LeaderCh():

			leaderChangesSeen.Inc()

			if l {
				s.shutdownLeader = make(chan struct{})

				go s.startLeader()

			} else {
				if s.shutdownLeader != nil {
					close(s.shutdownLeader)
				}
			}
		}
	}
}

func (s *Store) startLeader() {

	s.getPodsAndPushSecretIds()

	pollPodsTicker := time.NewTicker(s.config.PollPodsFrequency)

	watchSuccessful := true

	events, stop, err := s.kubeClient.WatchForPods()

	if err != nil {
		s.logger.Errorf("Could not watch pods: %s", err)
		watchSuccessful = false
	}

	for {
		select {
		case pod := <-events:
			s.Lock()
			if _, ok := s.pods[pod.Name]; !ok {
				s.pods[pod.Name] = pod

				go s.pushSecretIdToPod(pod)
			}
			s.Unlock()

		case <-pollPodsTicker.C:
			s.getPodsAndPushSecretIds()

			//Try to restart the watcher it it was not started successfully
			if !watchSuccessful {
				events, stop, err = s.kubeClient.WatchForPods()

				if err != nil {
					s.logger.Errorf("Could not watch pods: %s", err)
					watchSuccessful = false
				} else {
					watchSuccessful = true
				}
			}

		case <-s.shutdownLeader:
			s.logger.Debug("Shutting down leader.")
			pollPodsTicker.Stop()
			close(stop)
			return
		}
	}
}

func (s *Store) getPodsAndPushSecretIds() {
	pods, err := s.kubeClient.GetPods()

	if err != nil {
		s.logger.Errorf("Could not list pods: %s", err)
	}

	s.Lock()
	for _, pod := range pods {
		if _, ok := s.pods[pod.Name]; !ok {
			s.pods[pod.Name] = pod

			go s.pushSecretIdToPod(pod)
		}
	}
	s.Unlock()
}

func (s *Store) pushSecretIdToPod(pod client.Pod) {

	// Remove the pod from the list
	defer func() {
		s.Lock()
		delete(s.pods, pod.Name)
		s.Unlock()
	}()

	s.logger.Debugf("Attempting to push wrapped secret_id to pod (%s).", pod.Name)

	wrappedSecret, err := s.vaultClient.GetSecretId(pod.Role)

	if err != nil {
		s.logger.Errorf("Could not get secret_id for role (%s) for pod (%s): %s", pod.Role, pod.Name, err)
		return
	}

	b, err := json.Marshal(wrappedSecret)

	if err != nil {
		s.logger.Errorf("Could not marshal wrapped secret to JSON: %s", err)
		return
	}

	exp := backoff.NewExponentialBackOff()
	exp.MaxElapsedTime = maxHTTPPostTime

	// POST the secret with backoff
	op := func() error {

		ctx, _ := context.WithTimeout(context.Background(), HTTPPostTimeout)

		response, err := ctxhttp.Post(ctx, s.httpClient, fmt.Sprintf("https://%s:%d", pod.Ip, pod.Port), "application/json", bytes.NewReader(b))

		if err != nil {
			return errors.Wrap(err, "error POSTing wrapped token")
		}

		defer response.Body.Close()

		io.Copy(ioutil.Discard, response.Body)

		return nil
	}

	err = backoff.Retry(op, exp)
	secretPushes.With(prometheus.Labels{"approle": pod.Role}).Inc()

	if err != nil {
		secretPushFailures.With(prometheus.Labels{"approle": pod.Role}).Inc()
		s.logger.Errorf("Could not push wrapped secret_id to pod (%s): %s", pod.Name, err)
	} else {
		s.logger.Debugf("Successfully pushed wrapped secret_id to pod (%s)", pod.Name)
	}

	return
}

func (s *Store) handleGossipMembershipChange(memberEvent serf.MemberEvent) {
	peers, err := s.peerStore.Peers()

	if err != nil {
		s.logger.Errorf("Could not read from the peer store: %s", err)
		return
	}

	leader := s.Raft.VerifyLeader()

	for _, member := range memberEvent.Members {
		changedPeer := member.Addr.String() + ":" + strconv.Itoa(int(member.Port+1))

		if memberEvent.EventType() == serf.EventMemberJoin {

			nodeJoined.With(prometheus.Labels{"node": member.Addr.String()}).Inc()

			if leader.Error() == nil {
				f := s.Raft.AddPeer(changedPeer)

				if f.Error() != nil {
					s.logger.Errorf("Could not add peer to cluster using leader: %s", f.Error())
				}

			} else {
				newPeers := raft.AddUniquePeer(peers, changedPeer)
				f := s.Raft.SetPeers(newPeers)

				if f.Error() != nil {
					s.logger.Errorf("Could not add peer to list using follower: %s", f.Error())
				}
			}

		} else if memberEvent.EventType() == serf.EventMemberLeave || memberEvent.EventType() == serf.EventMemberFailed || memberEvent.EventType() == serf.EventMemberReap {

			switch memberEvent.EventType() {
			case serf.EventMemberLeave:
				nodeLeft.With(prometheus.Labels{"node": member.Addr.String()}).Inc()
			case serf.EventMemberFailed:
				nodeFailed.With(prometheus.Labels{"node": member.Addr.String()}).Inc()
			case serf.EventMemberReap:
				nodeReaped.With(prometheus.Labels{"node": member.Addr.String()}).Inc()
			}

			if leader.Error() == nil {

				f := s.Raft.RemovePeer(changedPeer)

				if f.Error() != nil {
					s.logger.Errorf("Could not remove peer from cluster using leader: %s", f.Error())
				}
			} else {
				newPeers := raft.ExcludePeer(peers, changedPeer)
				f := s.Raft.SetPeers(newPeers)

				if f.Error() != nil {
					s.logger.Errorf("Could not remove peer from list using follower: %s", f.Error())
				}
			}
		}
	}

	if peers, err := s.peerStore.Peers(); err != nil {
		s.logger.Errorf("Error getting peer list: %s", err)
	} else {
		nodesTotal.Set(float64(len(peers)))
	}
}

func (s *Store) Shutdown() {
	close(s.shutdown)
}

func NewStore(gossip *Gossip, kubeClient *client.Kube, vaultClient *client.Vault, config Config) *Store {

	// We need to skip TLS verification because the init container uses a self-signed and short-lived certificate
	// for secure communication.
	tr := cleanhttp.DefaultPooledTransport()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	return &Store{
		gossip:      gossip,
		config:      config,
		kubeClient:  kubeClient,
		vaultClient: vaultClient,
		httpClient:  &http.Client{Transport: tr},
		logger:      config.Logger,
		shutdown:    make(chan struct{}),
		pods:        map[string]client.Pod{},
	}
}
