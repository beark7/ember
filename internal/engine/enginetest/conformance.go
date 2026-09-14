// Package enginetest holds the conformance suite every engine adapter must
// pass: RunConformance exercises Prepare, Start, Health, Chat (stream
// contract), Tokenize, cancellation and Stop against a real Engine.
package enginetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/beark7/ember/internal/engine"
)

// RunConformance runs the adapter conformance suite. req must describe a
// small model the engine can load quickly (for the mock: a JSON descriptor;
// for llama.cpp: the tiny GGUF).
func RunConformance(t *testing.T, eng engine.Engine, req engine.PrepareRequest) {
	t.Helper()
	ctx := context.Background()

	if eng.Name() == "" {
		t.Fatal("Name() is empty")
	}

	spec, err := eng.Prepare(ctx, req)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if spec.Engine != eng.Name() {
		t.Fatalf("RunnerSpec.Engine = %q, want %q", spec.Engine, eng.Name())
	}

	r, err := eng.Start(ctx, spec)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })

	if r.ID() == "" {
		t.Fatal("ID() is empty")
	}
	if r.Endpoint() == "" {
		t.Fatal("Endpoint() is empty")
	}

	h, err := r.Health(ctx)
	if err != nil || !h.Ready {
		t.Fatalf("Health after Start = %+v, err %v; want ready", h, err)
	}
	if h.SlotsTotal < 1 || h.SlotsIdle < 1 {
		t.Fatalf("Health slots = %d/%d, want ≥ 1 idle", h.SlotsIdle, h.SlotsTotal)
	}

	t.Run("chat stream contract", func(t *testing.T) {
		ch, err := r.Chat(ctx, engine.ChatRequest{
			Messages: []engine.Message{engine.TextMessage(engine.RoleUser, "hello conformance world")},
			Sampling: engine.Sampling{MaxTokens: 8},
			Stream:   true,
		})
		if err != nil {
			t.Fatalf("Chat: %v", err)
		}
		var tokens, usages int
		var terminal *engine.Event
		for ev := range ch {
			if terminal != nil {
				t.Fatalf("event %v after terminal event", ev.Type)
			}
			switch ev.Type {
			case engine.EventToken:
				tokens++
			case engine.EventToolCall:
			case engine.EventUsage:
				usages++
				if ev.Usage == nil || ev.Usage.CompletionTokens <= 0 {
					t.Fatalf("usage event without counts: %+v", ev.Usage)
				}
			case engine.EventDone:
				e := ev
				terminal = &e
				if ev.FinishReason == "" {
					t.Fatal("done without finish reason")
				}
			case engine.EventError:
				t.Fatalf("unexpected error event: %v", ev.Err)
			}
		}
		if tokens == 0 {
			t.Fatal("no token events")
		}
		if usages != 1 {
			t.Fatalf("usage events = %d, want 1", usages)
		}
		if terminal == nil {
			t.Fatal("stream closed without terminal event")
		}
	})

	t.Run("tokenize", func(t *testing.T) {
		ids, err := r.Tokenize(ctx, "one two three")
		if err != nil {
			t.Fatalf("Tokenize: %v", err)
		}
		if len(ids) == 0 {
			t.Fatal("Tokenize returned no ids")
		}
	})

	t.Run("cancel closes stream promptly", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		ch, err := r.Chat(cctx, engine.ChatRequest{
			Messages: []engine.Message{engine.TextMessage(engine.RoleUser, "a very long answer please")},
			Sampling: engine.Sampling{MaxTokens: 100000},
			Stream:   true,
		})
		if err != nil {
			t.Fatalf("Chat: %v", err)
		}
		<-ch // wait for the first event so generation is under way
		cancel()
		deadline := time.After(500 * time.Millisecond)
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					return
				}
			case <-deadline:
				t.Fatal("stream not closed within 500ms after cancel")
			}
		}
	})

	t.Run("stop", func(t *testing.T) {
		if err := r.Stop(ctx); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		if err := r.Stop(ctx); err != nil {
			t.Fatalf("second Stop: %v", err)
		}
		if h, err := r.Health(ctx); err == nil && h.Ready {
			t.Fatal("Health ready after Stop")
		}
		if _, err := r.Chat(ctx, engine.ChatRequest{}); !errors.Is(err, engine.ErrStopped) {
			t.Fatalf("Chat after Stop: err = %v, want ErrStopped", err)
		}
	})
}
