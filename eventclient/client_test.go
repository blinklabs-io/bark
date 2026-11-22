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

// PublishBatch rejects any event whose type is "bad" and accepts the rest, so
// a test can see per-event failures come back beside successes.
func (f *fakeServer) PublishBatch(
	_ context.Context,
	req *connect.Request[eventv1alpha1.PublishBatchRequest],
) (*connect.Response[eventv1alpha1.PublishBatchResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var results []*eventv1alpha1.PublishBatchResult
	for _, ev := range req.Msg.GetEvents() {
		res := &eventv1alpha1.PublishBatchResult{EventId: ev.GetId()}
		if ev.GetEventType() == "bad" {
			res.Failure = &eventv1alpha1.PublishFailure{
				Code: eventv1alpha1.PublishFailureCode_PUBLISH_FAILURE_CODE_UNKNOWN_EVENT_TYPE,
			}
		} else {
			f.published = append(f.published, ev)
		}
		results = append(results, res)
	}
	return connect.NewResponse(
		&eventv1alpha1.PublishBatchResponse{Results: results},
	), nil
}

func (f *fakeServer) GetEventSchema(
	_ context.Context,
	req *connect.Request[eventv1alpha1.GetEventSchemaRequest],
) (*connect.Response[eventv1alpha1.GetEventSchemaResponse], error) {
	if req.Msg.GetEventType() != "chain.update" {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}
	return connect.NewResponse(&eventv1alpha1.GetEventSchemaResponse{
		Info: &eventv1alpha1.EventTypeInfo{
			Name:     "chain.update",
			Category: "chain",
		},
	}), nil
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

func runEvent(
	runID string,
	seq uint64,
) *eventv1alpha1.SubscribeWithAckResponse {
	resp := seqEvent(seq)
	resp.Event.RunId = runID
	return resp
}

func ready(
	runID string,
	seq uint64,
) *eventv1alpha1.SubscribeWithAckResponse {
	return &eventv1alpha1.SubscribeWithAckResponse{
		Ready: &eventv1alpha1.SubscribeWithAckReady{
			RunId:              runID,
			ResumeFromSequence: seq,
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

// TestPublishBatchReportsFailuresPerEvent checks that a rejected event shows
// up in its own result, in request order, while the call succeeds and the
// other events are delivered.
func TestPublishBatchReportsFailuresPerEvent(t *testing.T) {
	srv := &fakeServer{}
	c := newClient(t, srv)
	results, err := c.PublishBatch(context.Background(), []*eventv1alpha1.Event{
		{Id: "a", EventType: "chain.update"},
		{Id: "b", EventType: "bad"},
		{Id: "c", EventType: "mempool.add_tx"},
	})
	if err != nil {
		t.Fatalf("a rejected event failed the whole batch: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for i, wantID := range []string{"a", "b", "c"} {
		if results[i].GetEventId() != wantID {
			t.Errorf("result %d is for %q, want %q", i, results[i].GetEventId(), wantID)
		}
	}
	if results[0].GetFailure() != nil || results[2].GetFailure() != nil {
		t.Error("accepted events reported a failure")
	}
	if results[1].GetFailure().GetCode() !=
		eventv1alpha1.PublishFailureCode_PUBLISH_FAILURE_CODE_UNKNOWN_EVENT_TYPE {
		t.Errorf("rejected event failure = %v", results[1].GetFailure())
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.published) != 2 {
		t.Fatalf("server delivered %d events, want 2", len(srv.published))
	}
}

// TestGetEventSchema checks a known type's schema and that an unknown type
// surfaces as NotFound.
func TestGetEventSchema(t *testing.T) {
	c := newClient(t, &fakeServer{})
	got, err := c.GetEventSchema(context.Background(), "chain.update")
	if err != nil {
		t.Fatal(err)
	}
	if got.GetInfo().GetCategory() != "chain" {
		t.Fatalf("got %v", got)
	}
	_, err = c.GetEventSchema(context.Background(), "nope")
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("got %v, want NotFound", err)
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

// TestSubscribeWithAckKeepsInitialPositionAcrossReconnect checks that a
// disconnect before the first event does not move a new subscription's start
// position forward.
func TestSubscribeWithAckKeepsInitialPositionAcrossReconnect(t *testing.T) {
	srv := &fakeServer{}
	srv.ackSession = func(
		attempt int,
		start *eventv1alpha1.SubscribeWithAckStart,
		stream *connect.BidiStream[
			eventv1alpha1.SubscribeWithAckRequest,
			eventv1alpha1.SubscribeWithAckResponse,
		],
	) error {
		if attempt == 0 {
			if err := stream.Send(ready("run-1", 10)); err != nil {
				return err
			}
			return connect.NewError(connect.CodeUnavailable, errors.New("drop"))
		}
		if start.GetLastAckedRunId() != "run-1" ||
			start.GetLastAckedSequence() != 10 {
			return connect.NewError(
				connect.CodeInvalidArgument,
				errors.New("initial position changed"),
			)
		}
		if err := stream.Send(ready("run-1", 10)); err != nil {
			return err
		}
		if err := stream.Send(runEvent("run-1", 11)); err != nil {
			return err
		}
		if err := waitAck(stream, 11); err != nil {
			return err
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
	if want := []uint64{11}; !slices.Equal(got, want) {
		t.Fatalf("handled %v, want %v", got, want)
	}
}

// TestSubscribeWithAckResetsCursorForNewRun checks that a ready response from
// a new serving run replaces the previous run's larger sequence.
func TestSubscribeWithAckResetsCursorForNewRun(t *testing.T) {
	srv := &fakeServer{}
	srv.ackSession = func(
		_ int,
		start *eventv1alpha1.SubscribeWithAckStart,
		stream *connect.BidiStream[
			eventv1alpha1.SubscribeWithAckRequest,
			eventv1alpha1.SubscribeWithAckResponse,
		],
	) error {
		if start.GetLastAckedRunId() != "old-run" ||
			start.GetLastAckedSequence() != 42 {
			return connect.NewError(
				connect.CodeInvalidArgument,
				errors.New("resume cursor not forwarded"),
			)
		}
		if err := stream.Send(ready("new-run", 3)); err != nil {
			return err
		}
		if err := stream.Send(runEvent("new-run", 4)); err != nil {
			return err
		}
		if err := waitAck(stream, 4); err != nil {
			return err
		}
		return connect.NewError(connect.CodeOutOfRange, errors.New("done"))
	}
	c := newClient(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var got []*eventv1alpha1.Event
	err := c.SubscribeWithAck(
		ctx,
		eventclient.AckConfig{
			SubscriberID:      "s1",
			LastAckedRunID:    "old-run",
			LastAckedSequence: 42,
			InitialBackoff:    time.Millisecond,
		},
		func(_ context.Context, e *eventv1alpha1.Event) error {
			got = append(got, e)
			return nil
		},
	)
	if connect.CodeOf(err) != connect.CodeOutOfRange {
		t.Fatalf("got %v, want the server's terminal OUT_OF_RANGE", err)
	}
	if len(got) != 1 || got[0].GetRunId() != "new-run" ||
		got[0].GetSequence() != 4 {
		t.Fatalf("handled %v, want new-run sequence 4", got)
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

// TestSubscribeWithAckSendsOriginFilter checks that OriginNodeIDs reaches the
// server in the start message.
func TestSubscribeWithAckSendsOriginFilter(t *testing.T) {
	srv := &fakeServer{}
	srv.ackSession = func(
		_ int,
		_ *eventv1alpha1.SubscribeWithAckStart,
		_ *connect.BidiStream[
			eventv1alpha1.SubscribeWithAckRequest,
			eventv1alpha1.SubscribeWithAckResponse,
		],
	) error {
		return connect.NewError(connect.CodeOutOfRange, errors.New("done"))
	}
	c := newClient(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = c.SubscribeWithAck(
		ctx,
		eventclient.AckConfig{
			SubscriberID:  "o",
			EventTypes:    []string{"peergov.*"},
			OriginNodeIDs: []string{"node-b"},
		},
		func(context.Context, *eventv1alpha1.Event) error { return nil },
	)
	srv.mu.Lock()
	defer srv.mu.Unlock()
	f := srv.starts[0].GetFilter()
	if !slices.Equal(f.GetOriginNodeIds(), []string{"node-b"}) ||
		!slices.Equal(f.GetEventTypes(), []string{"peergov.*"}) {
		t.Fatalf("filter not forwarded: %v", f)
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

// TestSubscribeWithAckReturnsWhenCanceledWhileIdle checks that canceling the
// context ends a subscription that is waiting for its next event.
func TestSubscribeWithAckReturnsWhenCanceledWhileIdle(t *testing.T) {
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
		for {
			if _, err := stream.Receive(); err != nil {
				return nil
			}
		}
	}
	c := newClient(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	handled := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.SubscribeWithAck(
			ctx,
			eventclient.AckConfig{SubscriberID: "idle"},
			func(context.Context, *eventv1alpha1.Event) error {
				handled <- struct{}{}
				return nil
			},
		)
	}()
	<-handled
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SubscribeWithAck did not return after cancel")
	}
}
