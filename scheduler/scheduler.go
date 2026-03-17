package scheduler

import "github.com/OrlojHQ/orloj/resources"

// Node represents a compute target.
type Node struct {
	Name   string
	HasGPU bool
}

// Scheduler is a placeholder for phase-2 placement logic.
type Scheduler struct {
	nodes []Node
}

func New(nodes []Node) *Scheduler {
	return &Scheduler{nodes: nodes}
}

// Schedule returns the first node for MVP.
func (s *Scheduler) Schedule(_ resources.Agent) (Node, bool) {
	if len(s.nodes) == 0 {
		return Node{}, false
	}
	return s.nodes[0], true
}
