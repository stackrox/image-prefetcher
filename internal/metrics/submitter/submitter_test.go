package submitter

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stackrox/image-prefetcher/internal/metrics/gen"

	"github.com/neilotoole/slogt"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fakeClient struct {
	failures int
	calls    int
}

func (f *fakeClient) Submit(ctx context.Context, _ ...grpc.CallOption) (gen.Metrics_SubmitClient, error) {
	f.calls++
	if f.failures >= f.calls {
		return nil, fmt.Errorf("failing as requested, %d calls, %d faiulres", f.calls, f.failures)
	}
	return &fakeSubmitClient{}, nil
}

type fakeSubmitClient struct {
}

func (f *fakeSubmitClient) Send(result *gen.Result) error {
	return nil
}

func (f *fakeSubmitClient) CloseAndRecv() (*gen.Empty, error) {
	return nil, nil
}

func (f *fakeSubmitClient) Header() (metadata.MD, error) {
	panic("unimplemented")
}

func (f *fakeSubmitClient) Trailer() metadata.MD {
	panic("unimplemented")
}

func (f *fakeSubmitClient) CloseSend() error {
	panic("unimplemented")
}

func (f *fakeSubmitClient) Context() context.Context {
	panic("unimplemented")
}

func (f *fakeSubmitClient) SendMsg(m any) error {
	panic("unimplemented")
}

func (f *fakeSubmitClient) RecvMsg(m any) error {
	panic("unimplemented")
}

type testTimer struct {
	c chan time.Time
}

func (t *testTimer) Start(duration time.Duration) {
	go func() { t.c <- time.Now().Add(duration) }()
}

func (t *testTimer) Stop() {
}

func (t *testTimer) C() <-chan time.Time {
	return t.c
}

func TestSubmitter(t *testing.T) {
	tests := map[string]struct {
		client      *fakeClient
		expectCalls int
		timer       *testTimer
		timeout     time.Duration
		expectErr   error
	}{
		"nil": {
			client: nil,
		},
		"simple": {
			client:      &fakeClient{},
			expectCalls: 1,
		},
		"with retries": {
			client: &fakeClient{
				failures: 2,
			},
			timer:       &testTimer{},
			expectCalls: 3,
		},
		"timeout": {
			client: &fakeClient{
				failures: 999,
			},
			timeout:     50 * time.Millisecond,
			expectCalls: 1,
			expectErr:   context.DeadlineExceeded,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var sink *Submitter
			if test.client != nil {
				sink = NewSubmitter(slogt.New(t), test.client)
				if test.timer != nil {
					sink.timer = test.timer
				}
			}
			timeout := test.timeout
			if timeout == 0 {
				timeout = 30 * time.Second // to catch hangs early
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			func() {
				c := sink.Chan()
				if sink != nil {
					go func() { assert.ErrorIs(t, sink.Run(ctx), test.expectErr) }()
					c <- &gen.Result{Error: "bam"}
				}
				sink.Await()
				var actualCalls int
				if test.client != nil {
					actualCalls = test.client.calls
				}
				assert.Equal(t, test.expectCalls, actualCalls)
				defer cancel()
			}()
		})
	}
}
