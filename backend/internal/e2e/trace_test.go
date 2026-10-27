package e2e

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/gen/pidex/v1/pidexv1connect"
	"pidex/internal/tracing"
)

// TestChatTurnIsTraced: a turn is one trace, with the model call that read
// the message, the search it ran and the call that checked what was found,
// each with what it was given and gave back.
func TestChatTurnIsTraced(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tracing.Use(sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp)), true)
	t.Cleanup(func() { tracing.Use(sdktrace.NewTracerProvider(), false) })

	e := setup(t, "construction_renovation")
	maple := e.addSubject("project", "Maple Court", map[string]string{"project_name": "Maple Court"}, nil)
	e.addPhoto(fixturePhoto{subject: &maple, description: "Waterproofing membrane on the bathroom floor.", tags: []string{"waterproofing"}})
	chat := pidexv1connect.NewChatServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	e.searchAnswers.push(chatPlan("Maple Court", []string{"waterproofing"}, "photos", false))
	res, err := chat.SendMessage(e.ctx, connect.NewRequest(&pidexv1.SendMessageRequest{Text: "Maple Court waterproofing", TimeZone: "UTC"}))
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]tracetest.SpanStub{}
	for _, s := range exp.GetSpans() {
		byName[s.Name] = s
	}
	turn, ok := byName["chat turn"]
	if !ok {
		t.Fatalf("no turn traced; spans: %v", exp.GetSpans().Snapshots())
	}
	a := attrs(turn)
	if !strings.Contains(a["braintrust.input_json"], "Maple Court waterproofing") ||
		!strings.Contains(a["braintrust.metadata"], res.Msg.Conversation.Id) {
		t.Errorf("turn: input %s, metadata %s", a["braintrust.input_json"], a["braintrust.metadata"])
	}
	for _, name := range []string{"search_plan", "search photos", "search_check"} {
		s, ok := byName[name]
		if !ok {
			t.Errorf("no %q span", name)
			continue
		}
		if s.SpanContext.TraceID() != turn.SpanContext.TraceID() {
			t.Errorf("%q is not in the turn's trace", name)
		}
		if _, ok := attrs(s)["braintrust.output_json"]; !ok {
			t.Errorf("%q has no output", name)
		}
	}
}

func attrs(s tracetest.SpanStub) map[string]string {
	out := map[string]string{}
	for _, kv := range s.Attributes {
		out[string(kv.Key)] = kv.Value.Emit()
	}
	return out
}
