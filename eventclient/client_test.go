package eventclient_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/blinklabs-io/bark/eventclient"
	eventv1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/event"
	"github.com/blinklabs-io/bark/proto/v1alpha1/event/eventv1alpha1connect"
)

// fakeServer implements just enough of EventService to exercise the client.
type fakeServer struct {
	eventv1alpha1connect.UnimplementedEventServiceHandler

	mu        sync.Mutex
	published []*eventv1alpha1.Event
	starts    []*eventv1alpha1.SubscribeWithAckStart

	// ackSession runs one SubscribeWithAck connection. attempt counts
	// connections from zero.
	ackSession func(
		attempt int,
		start *eventv1alpha1.SubscribeWithAckStart,
		stream *connect.BidiStream[
			eventv1alpha1.SubscribeWithAckRequest,
			eventv1alpha1.SubscribeWithAckResponse,
		],
	) error
}

func (f *fakeServer) ListEventTypes(
	context.Context,
	*connect.Request[eventv1alpha1.ListEventTypesRequest],
) (*connect.Response[eventv1alpha1.ListEventTypesResponse], error) {
	return connect.NewResponse(&eventv1alpha1.ListEventTypesResponse{
		EventTypes: []*eventv1alpha1.EventTypeInfo{
			{Name: "chain.update"},
			{Name: "mempool.add_tx"},
		},
	}), nil
}

func (f *fakeServer) Publish(
	_ context.Context,
	req *connect.Request[eventv1alpha1.PublishRequest],
) (*connect.Response[eventv1alpha1.PublishResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, req.Msg.GetEvent())
	return connect.NewResponse(&eventv1alpha1.PublishResponse{}), nil
}

func (f *fakeServer) Subscribe(
	_ context.Context,
	req *connect.Request[eventv1alpha1.SubscribeRequest],
	stream *connect.ServerStream[eventv1alpha1.SubscribeResponse],
) error {
	for _, typ := range req.Msg.GetFilter().GetEventTypes() {
		err := stream.Send(&eventv1alpha1.SubscribeResponse{
			Event: &eventv1alpha1.Event{Id: typ, EventType: typ},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeServer) SubscribeWithAck(
	ctx context.Context,
	stream *connect.BidiStream[
		eventv1alpha1.SubscribeWithAckRequest,
		eventv1alpha1.SubscribeWithAckResponse,
	],
) error {
	first, err := stream.Receive()
	if err != nil {
		return err
	}
	start := first.GetStart()
	f.mu.Lock()
	attempt := len(f.starts)
	f.starts = append(f.starts, start)
	f.mu.Unlock()
	return f.ackSession(attempt, start, stream)
}

// newClient serves srv over HTTP/2 with TLS, which bidirectional streaming
// needs, and returns a client pointed at it.
func newClient(t *testing.T, srv *fakeServer) *eventclient.Client {
	t.Helper()
	_, handler := eventv1alpha1connect.NewEventServiceHandler(srv)
	ts := httptest.NewUnstartedServer(handler)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	t.Cleanup(ts.Close)
	return eventclient.New(ts.Client(), ts.URL)
}

// seqEvent returns a delivery of an event with the given sequence.
func seqEvent(seq uint64) *eventv1alpha1.SubscribeWithAckResponse {
	return &eventv1alpha1.SubscribeWithAckResponse{
		Event: &eventv1alpha1.Event{
			Id:        "e" + string(rune('0'+seq)),
			EventType: "chain.update",
			Sequence:  seq,
		},
	}
}

// waitAck blocks until the client has acknowledged want, so a test server can
// drop the connection only after the client's progress is on record.
func waitAck(
	stream *connect.BidiStream[
		eventv1alpha1.SubscribeWithAckRequest,
		eventv1alpha1.SubscribeWithAckResponse,
	],
	want uint64,
) error {
	for {
		msg, err := stream.Receive()
		if err != nil {
			return err
		}
		if msg.GetAck().GetSequence() >= want {
			return nil
		}
	}
}

// TestListEventTypes checks that the names in the server's registry reach the
// caller unchanged and in order.
func TestListEventTypes(t *testing.T) {
	c := newClient(t, &fakeServer{})
	got, err := c.ListEventTypes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"chain.update", "mempool.add_tx"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestPublish checks that the event the caller sends is the one the server's
// handler receives.
func TestPublish(t *testing.T) {
	srv := &fakeServer{}
	c := newClient(t, srv)
	ev := &eventv1alpha1.Event{Id: "x", EventType: "chain.update"}
	if err := c.Publish(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.published) != 1 || srv.published[0].GetId() != "x" {
		t.Fatalf("server saw %v", srv.published)
	}
}

// TestSubscribeReturnsNilWhenServerEndsStream checks that Subscribe delivers
// every event and reports a stream the server ended cleanly as success, not
// as an error.
func TestSubscribeReturnsNilWhenServerEndsStream(t *testing.T) {
	c := newClient(t, &fakeServer{})
	var got []string
	err := c.Subscribe(
		context.Background(),
		[]string{"a.one", "a.two"},
		func(e *eventv1alpha1.Event) { got = append(got, e.GetEventType()) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.one", "a.two"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestSubscribeWithAckResumesAfterDisconnect checks that a dropped connection
// loses nothing: the client reconnects with the last acknowledged sequence and
// the handler sees every event exactly once, in order.
func TestSubscribeWithAckResumesAfterDisconnect(t *testing.T) {
	srv := &fakeServer{}
	srv.ackSession = func(
		attempt int,
		start *eventv1alpha1.SubscribeWithAckStart,
		stream *connect.BidiStream[
			eventv1alpha1.SubscribeWithAckRequest,
			eventv1alpha1.SubscribeWithAckResponse,
		],
	) error {
		from, to := start.GetLastAckedSequence()+1, uint64(3)
		if attempt > 0 {
			to = 6
		}
		for seq := from; seq <= to; seq++ {
			if err := stream.Send(seqEvent(seq)); err != nil {
				return err
			}
		}
		if err := waitAck(stream, to); err != nil {
			return err
		}
		if attempt == 0 {
			return connect.NewError(connect.CodeUnavailable, errors.New("drop"))
		}
		return connect.NewError(connect.CodeOutOfRange, errors.New("done"))
	}
	c := newClient(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var got []uint64
	err := c.SubscribeWithAck(
		ctx,
		eventclient.AckConfig{
			SubscriberID:   "s1",
			InitialBackoff: time.Millisecond,
		},
		func(_ context.Context, e *eventv1alpha1.Event) error {
			got = append(got, e.GetSequence())
			return nil
		},
	)
	if connect.CodeOf(err) != connect.CodeOutOfRange {
		t.Fatalf("got %v, want the server's terminal OUT_OF_RANGE", err)
	}
	if want := []uint64{1, 2, 3, 4, 5, 6}; !slices.Equal(got, want) {
		t.Fatalf("handled %v, want %v", got, want)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.starts) < 2 || srv.starts[1].GetLastAckedSequence() != 3 {
		t.Fatalf("second start did not resume from 3: %v", srv.starts)
	}
	if srv.starts[1].GetSubscriberId() != "s1" {
		t.Fatalf("subscriber id changed: %v", srv.starts[1].GetSubscriberId())
	}
}

// TestSubscribeWithAckSkipsRedeliveredEvents checks that an event the server
// resends after a reconnect, which at-least-once delivery allows, does not
// reach the handler a second time.
func TestSubscribeWithAckSkipsRedeliveredEvents(t *testing.T) {
	srv := &fakeServer{}
	srv.ackSession = func(
		attempt int,
		_ *eventv1alpha1.SubscribeWithAckStart,
		stream *connect.BidiStream[
			eventv1alpha1.SubscribeWithAckRequest,
			eventv1alpha1.SubscribeWithAckResponse,
		],
	) error {
		seqs := []uint64{1, 2}
		if attempt > 0 {
			seqs = []uint64{2, 3}
		}
		for _, seq := range seqs {
			if err := stream.Send(seqEvent(seq)); err != nil {
				return err
			}
		}
		if err := waitAck(stream, seqs[len(seqs)-1]); err != nil {
			return err
		}
		if attempt == 0 {
			return connect.NewError(connect.CodeUnavailable, errors.New("drop"))
		}
		return connect.NewError(connect.CodeOutOfRange, errors.New("done"))
	}
	c := newClient(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var got []uint64
	err := c.SubscribeWithAck(
		ctx,
		eventclient.AckConfig{
			SubscriberID:   "s1",
			InitialBackoff: time.Millisecond,
		},
		func(_ context.Context, e *eventv1alpha1.Event) error {
			got = append(got, e.GetSequence())
			return nil
		},
	)
	if connect.CodeOf(err) != connect.CodeOutOfRange {
		t.Fatalf("got %v, want the server's terminal OUT_OF_RANGE", err)
	}
	if want := []uint64{1, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("handled %v, want %v", got, want)
	}
}

// TestSubscribeWithAckDoesNotRetryOutOfRange checks that a resume position the
// server no longer holds ends the subscription. Retrying would ask for the same
// position forever, and the caller has to start over.
func TestSubscribeWithAckDoesNotRetryOutOfRange(t *testing.T) {
	srv := &fakeServer{}
	srv.ackSession = func(
		int,
		*eventv1alpha1.SubscribeWithAckStart,
		*connect.BidiStream[
			eventv1alpha1.SubscribeWithAckRequest,
			eventv1alpha1.SubscribeWithAckResponse,
		],
	) error {
		return connect.NewError(connect.CodeOutOfRange, errors.New("gone"))
	}
	c := newClient(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := c.SubscribeWithAck(
		ctx,
		eventclient.AckConfig{
			SubscriberID:      "s1",
			LastAckedSequence: 9,
			InitialBackoff:    time.Millisecond,
		},
		func(context.Context, *eventv1alpha1.Event) error { return nil },
	)
	if connect.CodeOf(err) != connect.CodeOutOfRange {
		t.Fatalf("got %v, want OUT_OF_RANGE", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.starts) != 1 {
		t.Fatalf("retried %d times, want 1 attempt", len(srv.starts))
	}
}

// TestSubscribeWithAckStopsOnHandlerError checks that a handler error ends the
// subscription with that error and leaves the event unacknowledged, so the
// event is redelivered later instead of being silently dropped.
func TestSubscribeWithAckStopsOnHandlerError(t *testing.T) {
	srv := &fakeServer{}
	srv.ackSession = func(
		_ int,
		_ *eventv1alpha1.SubscribeWithAckStart,
		stream *connect.BidiStream[
			eventv1alpha1.SubscribeWithAckRequest,
			eventv1alpha1.SubscribeWithAckResponse,
		],
	) error {
		if err := stream.Send(seqEvent(1)); err != nil {
			return err
		}
		_, _ = stream.Receive()
		return nil
	}
	c := newClient(t, srv)

	boom := errors.New("boom")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := c.SubscribeWithAck(
		ctx,
		eventclient.AckConfig{
			SubscriberID:   "s1",
			InitialBackoff: time.Millisecond,
		},
		func(context.Context, *eventv1alpha1.Event) error { return boom },
	)
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want handler error", err)
	}
}

// TestSubscribeWithAckRequiresSubscriberID checks that a subscription without
// an ID is rejected, because the server could not tell which unacknowledged
// events to keep for it across a reconnect.
func TestSubscribeWithAckRequiresSubscriberID(t *testing.T) {
	c := newClient(t, &fakeServer{})
	err := c.SubscribeWithAck(
		context.Background(),
		eventclient.AckConfig{},
		func(context.Context, *eventv1alpha1.Event) error { return nil },
	)
	if err == nil {
		t.Fatal("expected an error")
	}
}
