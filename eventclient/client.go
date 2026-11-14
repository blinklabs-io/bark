// Package eventclient is a Go client for the EventService. It hides the
// stream handling so a consumer can subscribe, publish and list event types
// without writing the protocol itself.
package eventclient

import (
	"context"
	"errors"
	"io"
	"time"

	"connectrpc.com/connect"

	eventv1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/event"
	"github.com/blinklabs-io/bark/proto/v1alpha1/event/eventv1alpha1connect"
)

const (
	defaultInitialBackoff = time.Second
	defaultMaxBackoff     = 30 * time.Second
)

// Client calls an EventService.
type Client struct {
	svc eventv1alpha1connect.EventServiceClient
}

// New returns a Client for the EventService at baseURL.
func New(
	httpClient connect.HTTPClient,
	baseURL string,
	opts ...connect.ClientOption,
) *Client {
	return &Client{
		svc: eventv1alpha1connect.NewEventServiceClient(
			httpClient,
			baseURL,
			opts...,
		),
	}
}

// ListEventTypes returns the names of the event types the server accepts.
func (c *Client) ListEventTypes(ctx context.Context) ([]string, error) {
	resp, err := c.svc.ListEventTypes(
		ctx,
		connect.NewRequest(&eventv1alpha1.ListEventTypesRequest{}),
	)
	if err != nil {
		return nil, err
	}
	infos := resp.Msg.GetEventTypes()
	names := make([]string, 0, len(infos))
	for _, info := range infos {
		names = append(names, info.GetName())
	}
	return names, nil
}

// Publish delivers an event to the server's EventBus. A server may leave
// Publish disabled, in which case it returns an error.
func (c *Client) Publish(
	ctx context.Context,
	event *eventv1alpha1.Event,
) error {
	_, err := c.svc.Publish(
		ctx,
		connect.NewRequest(&eventv1alpha1.PublishRequest{Event: event}),
	)
	return err
}

// Subscribe calls handler for every event of the given types until ctx is
// done, the server ends the stream, or the stream fails. An empty eventTypes
// selects every registered type. Delivery is at-most-once: events published
// while the stream is down are lost, so Subscribe does not reconnect. It
// returns nil when the server ends the stream cleanly.
func (c *Client) Subscribe(
	ctx context.Context,
	eventTypes []string,
	handler func(*eventv1alpha1.Event),
) error {
	stream, err := c.svc.Subscribe(
		ctx,
		connect.NewRequest(&eventv1alpha1.SubscribeRequest{
			Filter: &eventv1alpha1.EventFilter{EventTypes: eventTypes},
		}),
	)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	for stream.Receive() {
		handler(stream.Msg().GetEvent())
	}
	return stream.Err()
}

// AckConfig configures SubscribeWithAck.
type AckConfig struct {
	// SubscriberID identifies the subscription across reconnects. It is
	// required.
	SubscriberID string
	// EventTypes selects the event types to receive. Empty selects every
	// registered type.
	EventTypes []string
	// LastAckedSequence is the sequence of the last event a previous run
	// processed, or zero to start from the stream's current position.
	LastAckedSequence uint64
	// InitialBackoff is the first wait before reconnecting. It doubles on
	// each consecutive failure up to MaxBackoff. Zero selects one second.
	InitialBackoff time.Duration
	// MaxBackoff caps the wait before reconnecting. Zero selects thirty
	// seconds.
	MaxBackoff time.Duration
}

// SubscribeWithAck calls handler for every event, in order, and acknowledges
// an event once handler returns nil for it. When the stream drops, it
// reconnects and resumes after the last acknowledged sequence, so a
// transient disconnect loses nothing. An event the server redelivers is
// skipped without calling handler again.
//
// It returns when ctx is done, when handler returns an error (the event is
// not acknowledged), or when the server rejects the subscription with a
// code that retrying cannot fix: INVALID_ARGUMENT, UNAUTHENTICATED,
// PERMISSION_DENIED, or OUT_OF_RANGE. After OUT_OF_RANGE the requested
// position is no longer buffered, and the caller must start over.
func (c *Client) SubscribeWithAck(
	ctx context.Context,
	cfg AckConfig,
	handler func(context.Context, *eventv1alpha1.Event) error,
) error {
	if cfg.SubscriberID == "" {
		return errors.New("eventclient: SubscriberID is required")
	}
	initial := cfg.InitialBackoff
	if initial <= 0 {
		initial = defaultInitialBackoff
	}
	maxBackoff := cfg.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = defaultMaxBackoff
	}
	last := cfg.LastAckedSequence
	backoff := initial
	for {
		progressed, err := c.ackSession(ctx, cfg, &last, handler)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !retryable(err) {
			var he *handlerError
			if errors.As(err, &he) {
				return he.err
			}
			return err
		}
		if progressed {
			backoff = initial
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

// ackSession runs one connection. It advances *last as events are
// acknowledged and reports whether it acknowledged any.
func (c *Client) ackSession(
	ctx context.Context,
	cfg AckConfig,
	last *uint64,
	handler func(context.Context, *eventv1alpha1.Event) error,
) (progressed bool, err error) {
	stream := c.svc.SubscribeWithAck(ctx)
	defer func() {
		_ = stream.CloseRequest()
		_ = stream.CloseResponse()
	}()
	err = stream.Send(&eventv1alpha1.SubscribeWithAckRequest{
		Message: &eventv1alpha1.SubscribeWithAckRequest_Start{
			Start: &eventv1alpha1.SubscribeWithAckStart{
				Filter: &eventv1alpha1.EventFilter{
					EventTypes: cfg.EventTypes,
				},
				SubscriberId:      cfg.SubscriberID,
				LastAckedSequence: *last,
			},
		},
	})
	if err != nil {
		return false, err
	}
	for {
		resp, recvErr := stream.Receive()
		if recvErr != nil {
			return progressed, recvErr
		}
		event := resp.GetEvent()
		if event.GetSequence() > *last {
			if handlerErr := handler(ctx, event); handlerErr != nil {
				return progressed, &handlerError{err: handlerErr}
			}
			*last = event.GetSequence()
			progressed = true
		}
		err = stream.Send(&eventv1alpha1.SubscribeWithAckRequest{
			Message: &eventv1alpha1.SubscribeWithAckRequest_Ack{
				Ack: &eventv1alpha1.SubscribeWithAckAck{Sequence: *last},
			},
		})
		if err != nil {
			return progressed, err
		}
	}
}

// handlerError marks an error returned by the caller's handler, which ends
// the subscription instead of triggering a reconnect.
type handlerError struct{ err error }

func (e *handlerError) Error() string { return e.err.Error() }

func (e *handlerError) Unwrap() error { return e.err }

// retryable reports whether a failed session should be retried. A clean end
// of stream and any transport failure are retried; a handler error and the
// codes that mean the request itself is wrong are not.
func retryable(err error) bool {
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}
	var he *handlerError
	if errors.As(err, &he) {
		return false
	}
	switch connect.CodeOf(err) {
	case connect.CodeInvalidArgument,
		connect.CodeUnauthenticated,
		connect.CodePermissionDenied,
		connect.CodeOutOfRange:
		return false
	default:
		return true
	}
}
