package resources

import "testing"

func TestContextAdapterNormalize(t *testing.T) {
	t.Parallel()
	ca := ContextAdapter{
		Metadata: ObjectMeta{Name: "tx-sanitize"},
		Spec: ContextAdapterSpec{
			ToolRef: "  my-tool  ",
			OnError: "",
		},
	}
	if err := ca.Normalize(); err != nil {
		t.Fatal(err)
	}
	if ca.Spec.ToolRef != "my-tool" || ca.Spec.OnError != "reject" {
		t.Fatalf("%#v %#v", ca.Spec.ToolRef, ca.Spec.OnError)
	}
	if ca.Status.Phase != "Ready" {
		t.Fatalf("expected Ready status, got %q", ca.Status.Phase)
	}
}
