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
	"github.com/nats-io/nats.go/jetstream"
)

const (
	defaultAgentMessageSubjectPrefix = "orloj.agentmsg"
	defaultAgentMessageStreamName    = "ORLOJ_AGENT_MESSAGES"

	// defaultStreamMaxBytes caps the stream at 1 GiB to prevent unbounded disk
	// growth when MaxAge alone is not enough (e.g. during message bursts).
	defaultStreamMaxBytes = 1 << 30 // 1 GiB

	// defaultConsumerMaxDeliver limits redelivery attempts for a single message.
	// After this many failures the message is terminated, preventing poison
	// messages from looping forever.
	defaultConsumerMaxDeliver = 10

	// defaultConsumerAckWait is the time the server waits for an ack before
	// redelivering.  Matches the long-running agent execution window.
	defaultConsumerAckWait = 120 * time.Second
)

type natsAgentMessageDelivery struct {
	msg     jetstream.Msg
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
	js            jetstream.JetStream
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

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream context: %w", err)
	}

	cfg := jetstream.StreamConfig{
		Name:      streamName,
		Subjects:  []string{subjectPrefix + ".>"},
		Storage:   jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		MaxBytes:  defaultStreamMaxBytes,
	}
	if _, err := js.CreateOrUpdateStream(context.Background(), cfg); err != nil {
		nc.Close()
		return nil, fmt.Errorf("create/update stream %q: %w", streamName, err)
	}

	bus := &NATSJetStreamAgentMessageBus{
		nc:            nc,
		js:            js,
		logger:        logger,
		subjectPrefix: subjectPrefix,
		streamName:    streamName,
	}
	if logger != nil {
		logger.Printf("agent message bus backend=nats-jetstream url=%s prefix=%s stream=%s max_bytes=%d", url, subjectPrefix, streamName, defaultStreamMaxBytes)
	}
	return bus, nil
}

func (b *NATSJetStreamAgentMessageBus) Publish(ctx context.Context, message AgentMessage) (AgentMessage, error) {
	normalized, err := normalizeAgentMessage(message)
	if err != nil {
		return AgentMessage{}, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return AgentMessage{}, err
	}
	subject := messageSubject(b.subjectPrefix, normalized.Namespace, normalized.ToAgent)
	_, err = b.js.Publish(ctx, subject, payload, jetstream.WithMsgID(normalized.MessageID))
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

	consumer, err := b.js.CreateOrUpdateConsumer(ctx, b.streamName, jetstream.ConsumerConfig{
		Durable:       durable,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       defaultConsumerAckWait,
		MaxDeliver:    defaultConsumerMaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("create/update consumer %q: %w", durable, err)
	}

	// Use the push-based Messages() iterator instead of the old Fetch polling
	// loop.  The server pushes messages and sends heartbeats so we consume zero
	// CPU when idle and get instant delivery when messages arrive.
	iter, err := consumer.Messages()
	if err != nil {
		return fmt.Errorf("consume messages %q: %w", durable, err)
	}
	defer iter.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msg, err := iter.Next()
		if err != nil {
			// iter.Stop() was called or context cancelled — exit cleanly.
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("message iterator error %q: %w", durable, err)
		}

		var payload AgentMessage
		if err := json.Unmarshal(msg.Data(), &payload); err != nil {
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

func (b *NATSJetStreamAgentMessageBus) Close() error {
	if b == nil || b.nc == nil {
		return nil
	}
	b.nc.Close()
	return nil
}
