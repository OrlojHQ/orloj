package resources

import "testing"

func TestSessionNormalizeDefaults(t *testing.T) {
	session := Session{
		Metadata: ObjectMeta{Name: "support-chat"},
		Spec:     SessionSpec{System: "support"},
	}
	if err := session.Normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if session.Kind != "Session" || session.APIVersion != "orloj.dev/v1" {
		t.Fatalf("identity defaults = %s %s", session.APIVersion, session.Kind)
	}
	if session.Metadata.Namespace != DefaultNamespace {
		t.Fatalf("namespace = %q", session.Metadata.Namespace)
	}
	if session.Spec.IdleTTL != "24h" {
		t.Fatalf("idle ttl = %q", session.Spec.IdleTTL)
	}
	if session.Status.Phase != SessionPhaseWaitingInput {
		t.Fatalf("phase = %q", session.Status.Phase)
	}
}

func TestSessionNormalizeRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		session Session
	}{
		{
			name:    "missing system",
			session: Session{Metadata: ObjectMeta{Name: "chat"}},
		},
		{
			name: "invalid ttl",
			session: Session{
				Metadata: ObjectMeta{Name: "chat"},
				Spec:     SessionSpec{System: "support", IdleTTL: "later"},
			},
		},
		{
			name: "negative turns",
			session: Session{
				Metadata: ObjectMeta{Name: "chat"},
				Spec:     SessionSpec{System: "support", MaxTurns: -1},
			},
		},
		{
			name: "invalid phase",
			session: Session{
				Metadata: ObjectMeta{Name: "chat"},
				Spec:     SessionSpec{System: "support"},
				Status:   SessionStatus{Phase: "Mystery"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.session.Normalize(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
