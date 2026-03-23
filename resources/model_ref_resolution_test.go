package resources

import (
	"context"
	"errors"
	"testing"
)

func TestParseModelEndpointRef(t *testing.T) {
	ns, name := ParseModelEndpointRef("default", "  my-endpoint ")
	if ns != "default" || name != "my-endpoint" {
		t.Fatalf("simple ref: ns=%q name=%q", ns, name)
	}
	ns, name = ParseModelEndpointRef("default", "team-a/anthropic")
	if ns != "team-a" || name != "anthropic" {
		t.Fatalf("scoped ref: ns=%q name=%q", ns, name)
	}
	ns, name = ParseModelEndpointRef("prod", "  other / model-1  ")
	if ns != "other" || name != "model-1" {
		t.Fatalf("trimmed scoped: ns=%q name=%q", ns, name)
	}
}

type fakeModelEndpoints struct {
	endpoint ModelEndpoint
	ok       bool
	err      error
}

func (f *fakeModelEndpoints) Get(ctx context.Context, name string) (ModelEndpoint, bool, error) {
	_ = ctx
	_ = name
	return f.endpoint, f.ok, f.err
}

func TestResolveAgentModelRef_Errors(t *testing.T) {
	ctx := context.Background()

	_, _, err := ResolveAgentModelRef(ctx, "default", "ref", nil)
	if err == nil || err.Error() == "" {
		t.Fatalf("expected error for nil lookup, got %v", err)
	}

	_, _, err = ResolveAgentModelRef(ctx, "default", "  ", &fakeModelEndpoints{})
	if err == nil {
		t.Fatal("expected error for empty model_ref")
	}

	_, _, err = ResolveAgentModelRef(ctx, "default", "missing", &fakeModelEndpoints{ok: false})
	if err == nil {
		t.Fatal("expected error when endpoint not found")
	}

	_, _, err = ResolveAgentModelRef(ctx, "default", "ep", &fakeModelEndpoints{
		ok:  true,
		err: errors.New("db down"),
	})
	if err == nil {
		t.Fatal("expected error from lookup failure")
	}

	_, _, err = ResolveAgentModelRef(ctx, "default", "ep", &fakeModelEndpoints{
		ok: true,
		endpoint: ModelEndpoint{
			Spec: ModelEndpointSpec{DefaultModel: ""},
		},
	})
	if err == nil {
		t.Fatal("expected error for empty default_model")
	}
}

func TestResolveAgentModelRef_Success(t *testing.T) {
	ctx := context.Background()
	ep := ModelEndpoint{
		Metadata: ObjectMeta{Name: "openai", Namespace: "default"},
		Spec:     ModelEndpointSpec{DefaultModel: "gpt-4o"},
	}
	lookup := &fakeModelEndpoints{
		ok:       true,
		endpoint: ep,
	}
	gotEP, model, err := ResolveAgentModelRef(ctx, "default", "openai", lookup)
	if err != nil {
		t.Fatal(err)
	}
	if model != "gpt-4o" {
		t.Fatalf("model: %q", model)
	}
	if gotEP.Spec.DefaultModel != "gpt-4o" {
		t.Fatalf("endpoint: %+v", gotEP.Spec)
	}
}
