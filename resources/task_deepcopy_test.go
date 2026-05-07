package resources

import "testing"

func TestDeepCopy_BlockedOnIsolation(t *testing.T) {
	original := Task{
		Status: TaskStatus{
			BlockedOn: &TaskBlockedOn{
				Kind:   "TaskApproval",
				Name:   "my-approval",
				Reason: "waiting for approval",
			},
		},
	}

	copied := original.DeepCopy()

	if copied.Status.BlockedOn == original.Status.BlockedOn {
		t.Fatal("DeepCopy shares BlockedOn pointer with original")
	}

	copied.Status.BlockedOn.Reason = "mutated"
	if original.Status.BlockedOn.Reason != "waiting for approval" {
		t.Fatalf("mutating copy's BlockedOn affected original: got %q", original.Status.BlockedOn.Reason)
	}
}

func TestDeepCopy_BlockedOnNil(t *testing.T) {
	original := Task{
		Status: TaskStatus{
			BlockedOn: nil,
		},
	}

	copied := original.DeepCopy()
	if copied.Status.BlockedOn != nil {
		t.Fatal("expected nil BlockedOn in copy")
	}
}
