package plumtree

import "time"

type PConfig struct {
	LazyPushInterval time.Duration
	LazyPushTimeout  time.Duration
}
