package cluster

import "time"

const (
	defaultGossipPort        = 45678
	defaultPollPodsFrequency = 20 * time.Second
	HTTPPostTimeout          = 30 * time.Second
	maxHTTPPostTime          = 3 * time.Minute
)
