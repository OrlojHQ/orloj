package resources

func (t Task) DeepCopy() Task {
	copy := t
	copy.Metadata = copyObjectMeta(t.Metadata)
	copy.Spec = copyTaskSpec(t.Spec)
	copy.Status = copyTaskStatus(t.Status)
	return copy
}

func copyObjectMeta(meta ObjectMeta) ObjectMeta {
	copy := meta
	copy.Labels = copyStringMap(meta.Labels)
	copy.Annotations = copyStringMap(meta.Annotations)
	return copy
}

func copyTaskSpec(spec TaskSpec) TaskSpec {
	copy := spec
	copy.Input = copyStringMap(spec.Input)
	if spec.MessageRetry.NonRetryable != nil {
		copy.MessageRetry.NonRetryable = append([]string(nil), spec.MessageRetry.NonRetryable...)
	}
	return copy
}

func copyTaskStatus(status TaskStatus) TaskStatus {
	copy := status
	copy.Output = copyStringMap(status.Output)
	copy.Trace = copyTaskTrace(status.Trace)
	copy.History = append([]TaskHistoryEvent(nil), status.History...)
	copy.Messages = append([]TaskMessage(nil), status.Messages...)
	copy.MessageIdempotency = append([]TaskMessageIdempotency(nil), status.MessageIdempotency...)
	copy.JoinStates = copyTaskJoinStates(status.JoinStates)
	copy.DelegationStates = copyTaskDelegationStates(status.DelegationStates)
	if status.BlockedOn != nil {
		blockedOn := *status.BlockedOn
		copy.BlockedOn = &blockedOn
	}
	return copy
}

func copyTaskTrace(events []TaskTraceEvent) []TaskTraceEvent {
	if events == nil {
		return nil
	}
	copy := make([]TaskTraceEvent, len(events))
	for i, event := range events {
		copy[i] = event
		if event.Retryable != nil {
			retryable := *event.Retryable
			copy[i].Retryable = &retryable
		}
	}
	return copy
}

func copyTaskJoinStates(states []TaskJoinState) []TaskJoinState {
	if states == nil {
		return nil
	}
	copy := make([]TaskJoinState, len(states))
	for i, state := range states {
		copy[i] = state
		copy[i].Sources = append([]TaskJoinSource(nil), state.Sources...)
	}
	return copy
}

func copyTaskDelegationStates(states []TaskDelegationState) []TaskDelegationState {
	if states == nil {
		return nil
	}
	copy := make([]TaskDelegationState, len(states))
	for i, state := range states {
		copy[i] = state
		copy[i].Sources = append([]TaskJoinSource(nil), state.Sources...)
	}
	return copy
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
