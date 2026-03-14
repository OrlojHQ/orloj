package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	defaultAgentMessageSubjectPrefix = "orloj.agentmsg"
	defaultAgentMessageStreamName    = "ORLOJ_AGENT_MESSAGES"
)

type natsAgentMessageDelivery struct {
	msg     *nats.Msg
	payload AgentMessage
	mu      sync.Mutex
	acked   bool
}

func (d *natsAgentMessageDelivery) Message() AgentMessage {
	return d.payload
}

func (d *natsAgentMessageDelivery) Ack(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.acked {
		return nil
	}
	d.acked = true
	return d.msg.Ack()
}

func (d *natsAgentMessageDelivery) Nack(_ context.Context, requeue bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.acked {
		return nil
	}
	d.acked = true
	if requeue {
		return d.msg.Nak()
	}
	return d.msg.Term()
}

func (d *natsAgentMessageDelivery) NackWithDelay(_ context.Context, delay time.Duration) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.acked {
		return nil
	}
	d.acked = true
	if delay <= 0 {
		return d.msg.Nak()
	}
	return d.msg.NakWithDelay(delay)
}

func (d *natsAgentMessageDelivery) ExtendLease(_ context.Context, _ time.Duration) error {
	return d.msg.InProgress()
}

// NATSJetStreamAgentMessageBus is a durable runtime message bus backed by JetStream.
type NATSJetStreamAgentMessageBus struct {
	nc            *nats.Conn
	js            nats.JetStreamContext
	logger        *log.Logger
	subjectPrefix string
	streamName    string
}

func NewNATSJetStreamAgentMessageBus(url string, subjectPrefix string, streamName string, logger *log.Logger) (*NATSJetStreamAgentMessageBus, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		url = nats.DefaultURL
	}
	subjectPrefix = strings.Trim(strings.TrimSpace(subjectPrefix), ".")
	if subjectPrefix == "" {
		subjectPrefix = defaultAgentMessageSubjectPrefix
	}
	streamName = strings.TrimSpace(streamName)
	if streamName == "" {
		streamName = defaultAgentMessageStreamName
	}

	nc, err := nats.Connect(
		url,
		nats.Name("orloj-agent-message-bus"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("connect nats %q: %w", url, err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream context: %w", err)
	}

	cfg := &nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{subjectPrefix + ".>"},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
	}
	if _, err := js.AddStream(cfg); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "stream name already in use") {
			nc.Close()
			return nil, fmt.Errorf("add stream %q: %w", streamName, err)
		}
		if logger != nil {
			logger.Printf("agent message stream already exists name=%s", streamName)
		}
	}

	bus := &NATSJetStreamAgentMessageBus{
		nc:            nc,
		js:            js,
		logger:        logger,
		subjectPrefix: subjectPrefix,
		streamName:    streamName,
	}
	if logger != nil {
		logger.Printf("agent message bus backend=nats-jetstream url=%s prefix=%s stream=%s", url, subjectPrefix, streamName)
	}
	return bus, nil
}

func (b *NATSJetStreamAgentMessageBus) Publish(_ context.Context, message AgentMessage) (AgentMessage, error) {
	normalized, err := normalizeAgentMessage(message)
	if err != nil {
		return AgentMessage{}, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return AgentMessage{}, err
	}
	subject := messageSubject(b.subjectPrefix, normalized.Namespace, normalized.ToAgent)
	_, err = b.js.Publish(subject, payload, nats.MsgId(normalized.MessageID))
	if err != nil {
		return AgentMessage{}, err
	}
	return normalized, nil
}

func (b *NATSJetStreamAgentMessageBus) Consume(ctx context.Context, sub AgentMessageSubscription, handler AgentMessageHandler) error {
	if handler == nil {
		return fmt.Errorf("handler is required")
	}
	subject := messageSubject(b.subjectPrefix, sub.Namespace, sub.Agent)
	durable := strings.TrimSpace(sub.Durable)
	if durable == "" {
		durable = fmt.Sprintf("agent-%s-%s", sanitizeSubjectToken(sub.Namespace), sanitizeSubjectToken(sub.Agent))
	}

	jetSub, err := b.js.PullSubscribe(
		subject,
		durable,
		nats.BindStream(b.streamName),
		nats.ManualAck(),
	)
	if err != nil {
		return err
	}
	defer jetSub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msgs, err := jetSub.Fetch(10, nats.MaxWait(2*time.Second))
		if err != nil {
			if err == nats.ErrTimeout || strings.Contains(strings.ToLower(err.Error()), "timeout") {
				continue
			}
			return err
		}
		for _, msg := range msgs {
			var payload AgentMessage
			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				if b.logger != nil {
					b.logger.Printf("agent message unmarshal failed: %v", err)
				}
				_ = msg.Term()
				continue
			}
			delivery := &natsAgentMessageDelivery{msg: msg, payload: payload}
			if err := handler(ctx, delivery); err != nil {
				if delay, ok := retryDelayFromError(err); ok {
					_ = delivery.NackWithDelay(ctx, delay)
				} else {
					_ = delivery.Nack(ctx, true)
				}
				continue
			}
			_ = delivery.Ack(ctx)
		}
	}
}

func (b *NATSJetStreamAgentMessageBus) Close() error {
	if b == nil || b.nc == nil {
		return nil
	}
	b.nc.Close()
	return nil
}
